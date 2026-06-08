package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func TestStoreInsertAndGetRun(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	run := pendingWriteRun("run_insert")

	// #when
	if err := store.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}
	got, err := store.GetRun(ctx, run.ID)

	// #then
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.ID != run.ID {
		t.Fatalf("ID = %q, want %q", got.ID, run.ID)
	}
	if got.ToolName != run.ToolName {
		t.Fatalf("ToolName = %q, want %q", got.ToolName, run.ToolName)
	}
	if got.Effect != run.Effect {
		t.Fatalf("Effect = %q, want %q", got.Effect, run.Effect)
	}
	if got.Status != run.Status {
		t.Fatalf("Status = %q, want %q", got.Status, run.Status)
	}
	if string(got.InputJSON) != string(run.InputJSON) {
		t.Fatalf("InputJSON = %s, want %s", got.InputJSON, run.InputJSON)
	}
	if !got.RequestedAt.Equal(run.RequestedAt) {
		t.Fatalf("RequestedAt = %s, want %s", got.RequestedAt, run.RequestedAt)
	}
	if got.ApprovedAt != nil || got.StartedAt != nil || got.FinishedAt != nil {
		t.Fatalf("optional timestamps = %#v %#v %#v, want nils", got.ApprovedAt, got.StartedAt, got.FinishedAt)
	}
}

func TestStoreGetRunMissingReturnsRunNotFound(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)

	// #when
	_, err := store.GetRun(ctx, "run_missing")

	// #then
	expectProtocolCode(t, err, protocol.RunNotFound)
}

func TestOpenReturnsStorageErrorWhenParentCannotBeCreated(t *testing.T) {
	// #given
	base := t.TempDir()
	blockingFile := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// #when
	store, err := Open(filepath.Join(blockingFile, "clix.db"))

	// #then
	if store != nil {
		t.Fatal("Open() store != nil, want nil")
	}
	expectProtocolCode(t, err, protocol.StorageError)
}

func TestOpenMigratesExistingRunsTableMissingToolSourcePath(t *testing.T) {
	// #given
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state", "clix.db")
	createLegacyRunsTable(t, dbPath)

	// #when
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	runs, err := store.ListRuns(ctx, nil)

	// #then
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListRuns() len = %d, want 1", len(runs))
	}
	if runs[0].ToolSourcePath != "" {
		t.Fatalf("ToolSourcePath = %q, want empty legacy value", runs[0].ToolSourcePath)
	}
}

func TestStoreListRunsFiltersByStatus(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	first := pendingWriteRun("run_pending_first")
	first.RequestedAt = testTime(2)
	second := pendingWriteRun("run_pending_second")
	second.RequestedAt = testTime(3)
	done := pendingWriteRun("run_done")
	done.RequestedAt = testTime(4)
	done.Status = domain.StatusSucceeded
	for _, run := range []domain.Run{first, second, done} {
		if err := store.InsertRun(ctx, run); err != nil {
			t.Fatalf("InsertRun(%s) error = %v", run.ID, err)
		}
	}

	// #when
	status := domain.StatusPendingApproval
	got, err := store.ListRuns(ctx, &status)

	// #then
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	wantIDs := []string{second.ID, first.ID}
	if len(got) != len(wantIDs) {
		t.Fatalf("ListRuns() len = %d, want %d", len(got), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("ListRuns()[%d].ID = %q, want %q", i, got[i].ID, wantID)
		}
	}
}

func TestStoreClaimPendingRunIsAtomic(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	run := pendingWriteRun("run_claim")
	if err := store.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}
	startedAt := testTime(5)

	// #when
	claimed, err := store.ClaimPendingRun(ctx, run.ID, startedAt)
	_, secondErr := store.ClaimPendingRun(ctx, run.ID, testTime(6))
	got, getErr := store.GetRun(ctx, run.ID)

	// #then
	if err != nil {
		t.Fatalf("first ClaimPendingRun() error = %v", err)
	}
	if claimed.Status != domain.StatusRunning {
		t.Fatalf("claimed status = %q, want %q", claimed.Status, domain.StatusRunning)
	}
	if claimed.StartedAt == nil || !claimed.StartedAt.Equal(startedAt) {
		t.Fatalf("claimed StartedAt = %#v, want %s", claimed.StartedAt, startedAt)
	}
	if claimed.ApprovedAt == nil || !claimed.ApprovedAt.Equal(startedAt) {
		t.Fatalf("claimed ApprovedAt = %#v, want %s", claimed.ApprovedAt, startedAt)
	}
	expectProtocolCode(t, secondErr, protocol.ApprovalError)
	if getErr != nil {
		t.Fatalf("GetRun() error = %v", getErr)
	}
	if got.Status != domain.StatusRunning {
		t.Fatalf("stored status = %q, want %q", got.Status, domain.StatusRunning)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
		t.Fatalf("stored StartedAt = %#v, want %s", got.StartedAt, startedAt)
	}
	if got.ApprovedAt == nil || !got.ApprovedAt.Equal(startedAt) {
		t.Fatalf("stored ApprovedAt = %#v, want %s", got.ApprovedAt, startedAt)
	}
}

func TestStoreClaimPendingRunRequiresWriteEffect(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	run := pendingWriteRun("run_read_pending")
	run.Effect = domain.EffectRead
	if err := store.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	// #when
	_, err := store.ClaimPendingRun(ctx, run.ID, testTime(14))

	// #then
	expectProtocolCode(t, err, protocol.ApprovalError)
}

func TestStoreMarksPendingAndTerminalStates(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	failed := pendingWriteRun("run_failed")
	failed.Status = domain.StatusCreated
	succeeded := pendingWriteRun("run_succeeded")
	if err := store.InsertRun(ctx, failed); err != nil {
		t.Fatalf("InsertRun(failed) error = %v", err)
	}
	if err := store.InsertRun(ctx, succeeded); err != nil {
		t.Fatalf("InsertRun(succeeded) error = %v", err)
	}

	// #when
	err := store.MarkPending(ctx, failed.ID)
	claimed, claimErr := store.ClaimPendingRun(ctx, failed.ID, testTime(7))
	failErr := store.MarkFailed(ctx, failed.ID, testTime(8), protocol.AdapterError, "adapter failed")
	claimedSucceeded, succeedClaimErr := store.ClaimPendingRun(ctx, succeeded.ID, testTime(9))
	succeedErr := store.MarkSucceeded(ctx, succeeded.ID, testTime(10))
	gotFailed, getFailedErr := store.GetRun(ctx, claimed.ID)
	gotSucceeded, getSucceededErr := store.GetRun(ctx, claimedSucceeded.ID)

	// #then
	if err != nil {
		t.Fatalf("MarkPending() error = %v", err)
	}
	if claimErr != nil {
		t.Fatalf("ClaimPendingRun(failed) error = %v", claimErr)
	}
	if failErr != nil {
		t.Fatalf("MarkFailed() error = %v", failErr)
	}
	if succeedClaimErr != nil {
		t.Fatalf("ClaimPendingRun(succeeded) error = %v", succeedClaimErr)
	}
	if succeedErr != nil {
		t.Fatalf("MarkSucceeded() error = %v", succeedErr)
	}
	if getFailedErr != nil || getSucceededErr != nil {
		t.Fatalf("GetRun() errors = %v %v", getFailedErr, getSucceededErr)
	}
	if gotFailed.Status != domain.StatusFailed {
		t.Fatalf("failed status = %q, want %q", gotFailed.Status, domain.StatusFailed)
	}
	if gotFailed.ExitCode == nil || *gotFailed.ExitCode != 1 {
		t.Fatalf("failed ExitCode = %#v, want 1", gotFailed.ExitCode)
	}
	if gotFailed.ErrorCode == nil || *gotFailed.ErrorCode != string(protocol.AdapterError) {
		t.Fatalf("failed ErrorCode = %#v, want %q", gotFailed.ErrorCode, protocol.AdapterError)
	}
	if gotFailed.ErrorMessage == nil || *gotFailed.ErrorMessage != "adapter failed" {
		t.Fatalf("failed ErrorMessage = %#v", gotFailed.ErrorMessage)
	}
	if !gotFailed.Status.Terminal() {
		t.Fatalf("%q should be terminal", gotFailed.Status)
	}
	if gotSucceeded.Status != domain.StatusSucceeded {
		t.Fatalf("succeeded status = %q, want %q", gotSucceeded.Status, domain.StatusSucceeded)
	}
	if gotSucceeded.ExitCode == nil || *gotSucceeded.ExitCode != 0 {
		t.Fatalf("succeeded ExitCode = %#v, want 0", gotSucceeded.ExitCode)
	}
}

func TestStoreMarkRejectedRequiresPendingApproval(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	pending := pendingWriteRun("run_reject_pending")
	running := pendingWriteRun("run_reject_running")
	if err := store.InsertRun(ctx, pending); err != nil {
		t.Fatalf("InsertRun(pending) error = %v", err)
	}
	if err := store.InsertRun(ctx, running); err != nil {
		t.Fatalf("InsertRun(running) error = %v", err)
	}
	if _, err := store.ClaimPendingRun(ctx, running.ID, testTime(11)); err != nil {
		t.Fatalf("ClaimPendingRun() error = %v", err)
	}

	// #when
	rejectErr := store.MarkRejected(ctx, pending.ID, testTime(12))
	invalidErr := store.MarkRejected(ctx, running.ID, testTime(13))
	got, getErr := store.GetRun(ctx, pending.ID)

	// #then
	if rejectErr != nil {
		t.Fatalf("MarkRejected(pending) error = %v", rejectErr)
	}
	expectProtocolCode(t, invalidErr, protocol.ApprovalError)
	if getErr != nil {
		t.Fatalf("GetRun() error = %v", getErr)
	}
	if got.Status != domain.StatusRejected {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusRejected)
	}
	if got.ErrorCode != nil {
		t.Fatalf("ErrorCode = %q, want nil", *got.ErrorCode)
	}
	if got.ErrorMessage != nil {
		t.Fatalf("ErrorMessage = %q, want nil", *got.ErrorMessage)
	}
}

func TestStoreMarkRejectedRequiresWriteEffect(t *testing.T) {
	// #given
	ctx := context.Background()
	store := openTestStore(t)
	run := pendingWriteRun("run_reject_read")
	run.Effect = domain.EffectRead
	if err := store.InsertRun(ctx, run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	// #when
	err := store.MarkRejected(ctx, run.ID, testTime(15))

	// #then
	expectProtocolCode(t, err, protocol.ApprovalError)
}

func TestRunPublicMapAndTerminalStatus(t *testing.T) {
	// #given
	run := pendingWriteRun("run_public")

	// #when
	base := run.PublicMap()
	code := string(protocol.AdapterError)
	message := "boom"
	run.ErrorCode = &code
	run.ErrorMessage = &message
	withError := run.PublicMap()

	// #then
	if base["id"] != run.ID {
		t.Fatalf("id = %#v, want %q", base["id"], run.ID)
	}
	if base["tool_name"] != run.ToolName {
		t.Fatalf("tool_name = %#v, want %q", base["tool_name"], run.ToolName)
	}
	if base["effect"] != string(run.Effect) {
		t.Fatalf("effect = %#v, want %q", base["effect"], run.Effect)
	}
	if base["status"] != string(run.Status) {
		t.Fatalf("status = %#v, want %q", base["status"], run.Status)
	}
	if base["requested_at"] != run.RequestedAt.Format(time.RFC3339Nano) {
		t.Fatalf("requested_at = %#v, want %q", base["requested_at"], run.RequestedAt.Format(time.RFC3339Nano))
	}
	if _, ok := base["error_code"]; ok {
		t.Fatal("error_code should be omitted when empty")
	}
	if withError["error_code"] != string(protocol.AdapterError) {
		t.Fatalf("error_code = %#v", withError["error_code"])
	}
	if withError["error_message"] != "boom" {
		t.Fatalf("error_message = %#v", withError["error_message"])
	}
	if domain.StatusCreated.Terminal() {
		t.Fatal("created should not be terminal")
	}
	if !domain.StatusSucceeded.Terminal() || !domain.StatusFailed.Terminal() || !domain.StatusRejected.Terminal() {
		t.Fatal("succeeded, failed, and rejected should be terminal")
	}
}

func openTestStore(t *testing.T) *SQLite {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "state", "clix.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func createLegacyRunsTable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE runs (
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

INSERT INTO runs (
  id, tool_name, effect, tool_fingerprint, input_json, status, requested_at
) VALUES (
  'run_legacy', 'weekly.submit_report', 'write', 'fingerprint-write',
  '{"week":"current","content":"done"}', 'pending_approval', '2026-06-05T12:01:00.123456789Z'
);
`)
	if err != nil {
		t.Fatalf("create legacy runs table error = %v", err)
	}
}

func pendingWriteRun(id string) domain.Run {
	return domain.Run{
		ID:              id,
		ToolName:        "weekly.submit_report",
		Effect:          domain.EffectWrite,
		ToolFingerprint: "fingerprint-write",
		ToolSourcePath:  filepath.Join("tools", "weekly.submit_report.yaml"),
		InputJSON:       []byte(`{"week":"current","content":"done"}`),
		Status:          domain.StatusPendingApproval,
		RequestedAt:     testTime(1),
	}
}

func testTime(offset int) time.Time {
	return time.Date(2026, 6, 5, 12, offset, 0, 123456789, time.UTC)
}

func expectProtocolCode(t *testing.T, err error, want protocol.Code) {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %T %v, want protocol error %s", err, err, want)
	}
	if perr.Code != want {
		t.Fatalf("error code = %q, want %q", perr.Code, want)
	}
}
