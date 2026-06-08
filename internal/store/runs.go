package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func (s *SQLite) InsertRun(ctx context.Context, run domain.Run) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runs (
  id, tool_name, effect, tool_fingerprint, tool_source_path, input_json, status,
  requested_at, approved_at, started_at, finished_at, exit_code, error_code, error_message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.ToolName,
		string(run.Effect),
		run.ToolFingerprint,
		run.ToolSourcePath,
		run.InputJSON,
		string(run.Status),
		formatTime(run.RequestedAt),
		formatOptionalTime(run.ApprovedAt),
		formatOptionalTime(run.StartedAt),
		formatOptionalTime(run.FinishedAt),
		optionalInt(run.ExitCode),
		optionalString(run.ErrorCode),
		optionalString(run.ErrorMessage),
	)
	if err != nil {
		return protocol.NewError(protocol.StorageError, err.Error())
	}
	return nil
}

func (s *SQLite) GetRun(ctx context.Context, id string) (domain.Run, error) {
	row := s.db.QueryRowContext(ctx, selectRunSQL+` WHERE id = ?`, id)
	run, err := scanRun(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Run{}, protocol.NewError(protocol.RunNotFound, "run not found")
		}
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	return run, nil
}

func (s *SQLite) ListRuns(ctx context.Context, status *domain.Status) ([]domain.Run, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if status == nil {
		rows, err = s.db.QueryContext(ctx, selectRunSQL+` ORDER BY requested_at DESC, id DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx, selectRunSQL+` WHERE status = ? ORDER BY requested_at DESC, id DESC`, string(*status))
	}
	if err != nil {
		return nil, protocol.NewError(protocol.StorageError, err.Error())
	}
	defer rows.Close()

	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, protocol.NewError(protocol.StorageError, err.Error())
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, protocol.NewError(protocol.StorageError, err.Error())
	}
	return runs, nil
}

func (s *SQLite) MarkPending(ctx context.Context, id string) error {
	return s.updateExisting(ctx, `
UPDATE runs
SET status = ?
WHERE id = ?`,
		string(domain.StatusPendingApproval), id)
}

func (s *SQLite) ClaimPendingRun(ctx context.Context, id string, startedAt time.Time) (domain.Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
UPDATE runs
SET status = ?, approved_at = ?, started_at = ?
WHERE id = ? AND status = ? AND effect = ?`,
		string(domain.StatusRunning),
		formatTime(startedAt),
		formatTime(startedAt),
		id,
		string(domain.StatusPendingApproval),
		string(domain.EffectWrite),
	)
	if err != nil {
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	if affected == 0 {
		if _, err := getRunTx(ctx, tx, id); err != nil {
			return domain.Run{}, err
		}
		return domain.Run{}, protocol.NewError(protocol.ApprovalError, "run is not pending approval")
	}

	run, err := getRunTx(ctx, tx, id)
	if err != nil {
		return domain.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	return run, nil
}

func (s *SQLite) MarkSucceeded(ctx context.Context, id string, finishedAt time.Time) error {
	exitCode := 0
	return s.updateExisting(ctx, `
UPDATE runs
SET status = ?, finished_at = ?, exit_code = ?, error_code = NULL, error_message = NULL
WHERE id = ?`,
		string(domain.StatusSucceeded), formatTime(finishedAt), exitCode, id)
}

func (s *SQLite) MarkFailed(ctx context.Context, id string, finishedAt time.Time, code protocol.Code, message string) error {
	exitCode := 1
	return s.updateExisting(ctx, `
UPDATE runs
SET status = ?, finished_at = ?, exit_code = ?, error_code = ?, error_message = ?
WHERE id = ?`,
		string(domain.StatusFailed), formatTime(finishedAt), exitCode, string(code), message, id)
}

func (s *SQLite) MarkRejected(ctx context.Context, id string, finishedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE runs
SET status = ?, finished_at = ?
WHERE id = ? AND status = ? AND effect = ?`,
		string(domain.StatusRejected),
		formatTime(finishedAt),
		id,
		string(domain.StatusPendingApproval),
		string(domain.EffectWrite),
	)
	if err != nil {
		return protocol.NewError(protocol.StorageError, err.Error())
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return protocol.NewError(protocol.StorageError, err.Error())
	}
	if affected > 0 {
		return nil
	}
	if _, err := s.GetRun(ctx, id); err != nil {
		return err
	}
	return protocol.NewError(protocol.ApprovalError, "run is not pending approval")
}

func (s *SQLite) updateExisting(ctx context.Context, query string, args ...any) error {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return protocol.NewError(protocol.StorageError, err.Error())
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return protocol.NewError(protocol.StorageError, err.Error())
	}
	if affected == 0 {
		return protocol.NewError(protocol.RunNotFound, "run not found")
	}
	return nil
}

const selectRunSQL = `
SELECT id, tool_name, effect, tool_fingerprint, tool_source_path, input_json, status,
       requested_at, approved_at, started_at, finished_at, exit_code, error_code, error_message
FROM runs`

type rowScanner interface {
	Scan(dest ...any) error
}

func getRunTx(ctx context.Context, tx *sql.Tx, id string) (domain.Run, error) {
	row := tx.QueryRowContext(ctx, selectRunSQL+` WHERE id = ?`, id)
	run, err := scanRun(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Run{}, protocol.NewError(protocol.RunNotFound, "run not found")
		}
		return domain.Run{}, protocol.NewError(protocol.StorageError, err.Error())
	}
	return run, nil
}

func scanRun(row rowScanner) (domain.Run, error) {
	var (
		run                                     domain.Run
		effectText, statusText, requestedText   string
		approvedText, startedText, finishedText sql.NullString
		exitCode                                sql.NullInt64
		errorCode, errorMessage                 sql.NullString
	)
	err := row.Scan(
		&run.ID,
		&run.ToolName,
		&effectText,
		&run.ToolFingerprint,
		&run.ToolSourcePath,
		&run.InputJSON,
		&statusText,
		&requestedText,
		&approvedText,
		&startedText,
		&finishedText,
		&exitCode,
		&errorCode,
		&errorMessage,
	)
	if err != nil {
		return domain.Run{}, err
	}
	requestedAt, err := parseTime(requestedText)
	if err != nil {
		return domain.Run{}, err
	}
	run.Effect = domain.Effect(effectText)
	run.Status = domain.Status(statusText)
	run.RequestedAt = requestedAt
	run.ApprovedAt, err = parseOptionalTime(approvedText)
	if err != nil {
		return domain.Run{}, err
	}
	run.StartedAt, err = parseOptionalTime(startedText)
	if err != nil {
		return domain.Run{}, err
	}
	run.FinishedAt, err = parseOptionalTime(finishedText)
	if err != nil {
		return domain.Run{}, err
	}
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	if errorCode.Valid {
		run.ErrorCode = &errorCode.String
	}
	if errorMessage.Valid {
		run.ErrorMessage = &errorMessage.String
	}
	return run, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*value), Valid: true}
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func optionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
