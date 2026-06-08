package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/runservice"
)

func TestApproveCommandPrintsTextRow(t *testing.T) {
	// #given
	service := &fakeRunService{
		approveResult: runservice.Result{Run: writeRun(domain.StatusSucceeded), Output: json.RawMessage(`{"ok":true}`)},
	}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"approve", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "run_write succeeded weekly.submit_report\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if service.approveID != "run_write" {
		t.Fatalf("Approve() id = %q", service.approveID)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestApproveCommandProjectsJSONOutput(t *testing.T) {
	// #given
	service := &fakeRunService{
		approveResult: runservice.Result{Run: writeRun(domain.StatusSucceeded), Output: json.RawMessage(`{"ok":true}`)},
	}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status,output", "approve", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\"id\":\"run_write\",\"output\":{\"ok\":true},\"status\":\"succeeded\"}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRejectCommandPrintsTextRow(t *testing.T) {
	// #given
	service := &fakeRunService{rejectResult: writeRun(domain.StatusRejected)}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"reject", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "run_write rejected weekly.submit_report\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if service.rejectID != "run_write" {
		t.Fatalf("Reject() id = %q", service.rejectID)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRejectCommandProjectsJSONOutput(t *testing.T) {
	// #given
	service := &fakeRunService{rejectResult: writeRun(domain.StatusRejected)}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status", "reject", "run_write"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\"id\":\"run_write\",\"status\":\"rejected\"}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func writeRun(status domain.Status) domain.Run {
	return domain.Run{
		ID:          "run_write",
		ToolName:    "weekly.submit_report",
		Effect:      domain.EffectWrite,
		Status:      status,
		RequestedAt: testRunTime(),
	}
}
