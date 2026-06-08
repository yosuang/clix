package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

func TestRunsListPrintsTextRows(t *testing.T) {
	// #given
	store := &fakeRunStore{listRuns: []domain.Run{storedWriteRun(), storedReadRun()}}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"runs", "list"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "run_write pending_approval weekly.submit_report\nrun_read succeeded weekly.get_records\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if store.listStatus != nil {
		t.Fatalf("ListRuns() status = %v, want nil", *store.listStatus)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsListFiltersByStatus(t *testing.T) {
	// #given
	store := &fakeRunStore{listRuns: []domain.Run{storedWriteRun()}}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"runs", "list", "--status", "pending_approval"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.listStatus == nil || *store.listStatus != domain.StatusPendingApproval {
		t.Fatalf("ListRuns() status = %#v, want %q", store.listStatus, domain.StatusPendingApproval)
	}
	want := "run_write pending_approval weekly.submit_report\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsListRejectsUnknownStatus(t *testing.T) {
	// #given
	store := &fakeRunStore{}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"runs", "list", "--status", "missing"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != `USAGE_ERROR: unknown status "missing"` {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.listCalled {
		t.Fatal("ListRuns() was called for invalid status")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsListProjectsJSONRows(t *testing.T) {
	// #given
	store := &fakeRunStore{listRuns: []domain.Run{storedWriteRun(), storedReadRun()}}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status,tool_name", "runs", "list"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "[{\"id\":\"run_write\",\"status\":\"pending_approval\",\"tool_name\":\"weekly.submit_report\"},{\"id\":\"run_read\",\"status\":\"succeeded\",\"tool_name\":\"weekly.get_records\"}]\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsGetPrintsTextRow(t *testing.T) {
	// #given
	store := &fakeRunStore{getRun: storedWriteRun()}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"runs", "get", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.getID != "run_write" {
		t.Fatalf("GetRun() id = %q", store.getID)
	}
	want := "run_write pending_approval weekly.submit_report\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsGetProjectsJSONMetadata(t *testing.T) {
	// #given
	store := &fakeRunStore{getRun: storedWriteRun()}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status,error_code,error_message", "runs", "get", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\"error_code\":null,\"error_message\":null,\"id\":\"run_write\",\"status\":\"pending_approval\"}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunsGetProjectsInputOnlyWhenRequested(t *testing.T) {
	// #given
	store := &fakeRunStore{getRun: storedWriteRun()}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunStore: store}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,input", "runs", "get", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\"id\":\"run_write\",\"input\":{\"content\":\"done\",\"week\":\"current\"}}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

type fakeRunStore struct {
	listRuns   []domain.Run
	getRun     domain.Run
	listCalled bool
	listStatus *domain.Status
	getID      string
}

func (s *fakeRunStore) InsertRun(context.Context, domain.Run) error {
	panic("InsertRun should not be called")
}

func (s *fakeRunStore) GetRun(_ context.Context, id string) (domain.Run, error) {
	s.getID = id
	return s.getRun, nil
}

func (s *fakeRunStore) ListRuns(_ context.Context, status *domain.Status) ([]domain.Run, error) {
	s.listCalled = true
	if status != nil {
		copied := *status
		s.listStatus = &copied
	}
	return s.listRuns, nil
}

func (s *fakeRunStore) ClaimPendingRun(context.Context, string, time.Time) (domain.Run, error) {
	panic("ClaimPendingRun should not be called")
}

func (s *fakeRunStore) MarkSucceeded(context.Context, string, time.Time) error {
	panic("MarkSucceeded should not be called")
}

func (s *fakeRunStore) MarkFailed(context.Context, string, time.Time, protocol.Code, string) error {
	panic("MarkFailed should not be called")
}

func (s *fakeRunStore) MarkRejected(context.Context, string, time.Time) error {
	panic("MarkRejected should not be called")
}

func storedWriteRun() domain.Run {
	run := writeRun(domain.StatusPendingApproval)
	run.InputJSON = []byte(`{"content":"done","week":"current"}`)
	return run
}

func storedReadRun() domain.Run {
	run := readRun()
	run.InputJSON = []byte(`{"week":"current"}`)
	return run
}
