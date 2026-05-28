package clix

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	// Register the sqlite3 database/sql driver used by openStore.
	_ "github.com/mattn/go-sqlite3"
)

const (
	StatusCreated         = "created"
	StatusFailed          = "failed"
	StatusPendingApproval = "pending_approval"
	StatusRejected        = "rejected"
	StatusRunning         = "running"
	StatusSucceeded       = "succeeded"
)

type Store struct {
	db *sql.DB
}

type RunRecord struct {
	ID              string  `json:"id"`
	ToolName        string  `json:"tool_name"`
	Effect          string  `json:"effect"`
	ToolFingerprint string  `json:"tool_fingerprint"`
	InputJSON       string  `json:"input_json"`
	Status          string  `json:"status"`
	RequestedAt     string  `json:"requested_at"`
	ApprovedAt      *string `json:"approved_at"`
	StartedAt       *string `json:"started_at"`
	FinishedAt      *string `json:"finished_at"`
	ExitCode        *int    `json:"exit_code"`
	ErrorCode       *string `json:"error_code"`
	ErrorMessage    *string `json:"error_message"`
}

func openStore(path string) (*Store, *AppError) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errorf(CodeStorageError, "create database directory: %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, errorf(CodeStorageError, "open database: %v", err)
	}
	store := &Store{db: db}
	if appErr := store.init(); appErr != nil {
		_ = db.Close()
		return nil, appErr
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() *AppError {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  tool_name TEXT NOT NULL,
  effect TEXT NOT NULL,
  tool_fingerprint TEXT NOT NULL,
  input_json TEXT NOT NULL,
  status TEXT NOT NULL,
  requested_at TEXT NOT NULL,
  approved_at TEXT,
  started_at TEXT,
  finished_at TEXT,
  exit_code INTEGER,
  error_code TEXT,
  error_message TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_requested_at ON runs(requested_at);
`)
	if err != nil {
		return errorf(CodeStorageError, "initialize database: %v", err)
	}
	return nil
}

func (s *Store) createRun(record RunRecord) *AppError {
	_, err := s.db.Exec(`
INSERT INTO runs (
  id, tool_name, effect, tool_fingerprint, input_json, status, requested_at,
  approved_at, started_at, finished_at, exit_code, error_code, error_message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.ToolName,
		record.Effect,
		record.ToolFingerprint,
		record.InputJSON,
		record.Status,
		record.RequestedAt,
		record.ApprovedAt,
		record.StartedAt,
		record.FinishedAt,
		record.ExitCode,
		record.ErrorCode,
		record.ErrorMessage,
	)
	if err != nil {
		return errorf(CodeStorageError, "create run: %v", err)
	}
	return nil
}

func (s *Store) markPending(id string) (RunRecord, *AppError) {
	result, err := s.db.Exec(`UPDATE runs SET status = ? WHERE id = ? AND status = ?`, StatusPendingApproval, id, StatusCreated)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "mark run pending: %v", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "mark run pending: %v", err)
	}
	if changed != 1 {
		return RunRecord{}, errorf(CodeApprovalError, "run %q cannot become pending from its current status", id)
	}
	return s.getRun(id)
}

func (s *Store) markRunning(id, startedAt string) (RunRecord, *AppError) {
	result, err := s.db.Exec(
		`UPDATE runs SET status = ?, started_at = ? WHERE id = ? AND status = ?`,
		StatusRunning,
		startedAt,
		id,
		StatusCreated,
	)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "mark run running: %v", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "mark run running: %v", err)
	}
	if changed != 1 {
		return RunRecord{}, errorf(CodeApprovalError, "run %q cannot start from its current status", id)
	}
	return s.getRun(id)
}

func (s *Store) approveRun(ctx context.Context, id, expectedFingerprint, approvedAt, startedAt string) (RunRecord, *AppError) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "begin approval transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	record, appErr := scanRun(tx.QueryRowContext(ctx, selectRunSQL+` WHERE id = ?`, id))
	if appErr != nil {
		return RunRecord{}, appErr
	}
	if record.Status != StatusPendingApproval {
		return RunRecord{}, errorf(CodeApprovalError, "run %q is %s, not pending_approval", id, record.Status)
	}
	if record.ToolFingerprint != expectedFingerprint {
		finishedAt := approvedAt
		exitCode := 1
		errorCode := CodeManifestChanged
		errorMessage := "tool definition changed after the run was created"
		_, err := tx.ExecContext(ctx, `
UPDATE runs
SET status = ?, finished_at = ?, exit_code = ?, error_code = ?, error_message = ?
WHERE id = ? AND status = ?`,
			StatusRejected,
			finishedAt,
			exitCode,
			errorCode,
			errorMessage,
			id,
			StatusPendingApproval,
		)
		if err != nil {
			return RunRecord{}, errorf(CodeStorageError, "reject changed run: %v", err)
		}
		if err := tx.Commit(); err != nil {
			return RunRecord{}, errorf(CodeStorageError, "commit changed run rejection: %v", err)
		}
		record.Status = StatusRejected
		record.FinishedAt = &finishedAt
		record.ExitCode = &exitCode
		record.ErrorCode = &errorCode
		record.ErrorMessage = &errorMessage
		return record, newError(CodeManifestChanged, errorMessage)
	}

	result, err := tx.ExecContext(ctx, `
UPDATE runs
SET status = ?, approved_at = ?, started_at = ?, exit_code = NULL, error_code = NULL, error_message = NULL
WHERE id = ? AND status = ?`,
		StatusRunning,
		approvedAt,
		startedAt,
		id,
		StatusPendingApproval,
	)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "approve run: %v", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "approve run: %v", err)
	}
	if changed != 1 {
		return RunRecord{}, errorf(CodeApprovalError, "run %q was already claimed", id)
	}
	if err := tx.Commit(); err != nil {
		return RunRecord{}, errorf(CodeStorageError, "commit approval: %v", err)
	}
	record.Status = StatusRunning
	record.ApprovedAt = &approvedAt
	record.StartedAt = &startedAt
	record.ExitCode = nil
	record.ErrorCode = nil
	record.ErrorMessage = nil
	return record, nil
}

func (s *Store) completeRun(id, status string, exitCode int, errorCode, errorMessage *string, finishedAt string) (RunRecord, *AppError) {
	result, err := s.db.Exec(`
UPDATE runs
SET status = ?, finished_at = ?, exit_code = ?, error_code = ?, error_message = ?
WHERE id = ? AND status = ?`,
		status,
		finishedAt,
		exitCode,
		errorCode,
		errorMessage,
		id,
		StatusRunning,
	)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "complete run: %v", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "complete run: %v", err)
	}
	if changed != 1 {
		return RunRecord{}, errorf(CodeStorageError, "run %q was not running", id)
	}
	return s.getRun(id)
}

func (s *Store) rejectRun(id, finishedAt string) (RunRecord, *AppError) {
	exitCode := 0
	result, err := s.db.Exec(`
UPDATE runs
SET status = ?, finished_at = ?, exit_code = ?
WHERE id = ? AND status = ?`,
		StatusRejected,
		finishedAt,
		exitCode,
		id,
		StatusPendingApproval,
	)
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "reject run: %v", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "reject run: %v", err)
	}
	if changed != 1 {
		record, appErr := s.getRun(id)
		if appErr != nil {
			return RunRecord{}, appErr
		}
		return RunRecord{}, errorf(CodeApprovalError, "run %q is %s, not pending_approval", id, record.Status)
	}
	return s.getRun(id)
}

func (s *Store) getRun(id string) (RunRecord, *AppError) {
	return scanRun(s.db.QueryRow(selectRunSQL+` WHERE id = ?`, id))
}

func (s *Store) listRuns(status string) ([]RunRecord, *AppError) {
	query := selectRunSQL
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY requested_at DESC, id DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, errorf(CodeStorageError, "list runs: %v", err)
	}
	defer rows.Close()
	var records []RunRecord
	for rows.Next() {
		record, appErr := scanRun(rows)
		if appErr != nil {
			return nil, appErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, errorf(CodeStorageError, "list runs: %v", err)
	}
	return records, nil
}

const selectRunSQL = `
SELECT id, tool_name, effect, tool_fingerprint, input_json, status, requested_at,
       approved_at, started_at, finished_at, exit_code, error_code, error_message
FROM runs`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (RunRecord, *AppError) {
	var record RunRecord
	var approvedAt, startedAt, finishedAt sql.NullString
	var exitCode sql.NullInt64
	var errorCode, errorMessage sql.NullString
	err := row.Scan(
		&record.ID,
		&record.ToolName,
		&record.Effect,
		&record.ToolFingerprint,
		&record.InputJSON,
		&record.Status,
		&record.RequestedAt,
		&approvedAt,
		&startedAt,
		&finishedAt,
		&exitCode,
		&errorCode,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RunRecord{}, newError(CodeRunNotFound, "run not found")
	}
	if err != nil {
		return RunRecord{}, errorf(CodeStorageError, "read run: %v", err)
	}
	record.ApprovedAt = nullStringPtr(approvedAt)
	record.StartedAt = nullStringPtr(startedAt)
	record.FinishedAt = nullStringPtr(finishedAt)
	if exitCode.Valid {
		value := int(exitCode.Int64)
		record.ExitCode = &value
	}
	record.ErrorCode = nullStringPtr(errorCode)
	record.ErrorMessage = nullStringPtr(errorMessage)
	return record, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
