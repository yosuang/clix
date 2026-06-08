package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/runservice"
)

func TestInputSourceUsesInputFlagOverTTYStdin(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: `{"week":"current"}`, InputSet: true, StdinTTY: true}

	// #when
	got, err := opts.InputReader(strings.NewReader(""))

	// #then
	if err != nil {
		t.Fatalf("InputReader() error = %v", err)
	}
	body, _ := io.ReadAll(got)
	if string(body) != `{"week":"current"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestInputSourceRejectsInputFlagWithNonEmptyPipe(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: `{"week":"current"}`, InputSet: true, StdinTTY: false}

	// #when
	_, err := opts.InputReader(strings.NewReader(`{"other":true}`))

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --input cannot be combined with non-empty stdin" {
		t.Fatalf("InputReader() error = %v", err)
	}
}

func TestInputSourceRejectsMissingTTYInput(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: "", StdinTTY: true}

	// #when
	_, err := opts.InputReader(strings.NewReader(""))

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input is required" {
		t.Fatalf("InputReader() error = %v", err)
	}
}

func TestInputSourceUsesExplicitEmptyInputFlag(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: "", InputSet: true, StdinTTY: true}

	// #when
	got, err := opts.InputReader(strings.NewReader(""))

	// #then
	if err != nil {
		t.Fatalf("InputReader() error = %v", err)
	}
	body, _ := io.ReadAll(got)
	if string(body) != "" {
		t.Fatalf("body = %q, want empty input flag", body)
	}
}

func TestRunCommandRunsToolWithInputFlag(t *testing.T) {
	// #given
	service := &fakeRunService{
		runResult: runservice.Result{Run: readRun(), Output: json.RawMessage(`{"records":[]}`)},
	}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"run", "weekly.get_records", "--input", `{"week":"current"}`})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "run_read succeeded weekly.get_records\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if service.runToolName != "weekly.get_records" {
		t.Fatalf("Run() tool name = %q", service.runToolName)
	}
	if string(service.runInput) != `{"week":"current"}` {
		t.Fatalf("Run() input = %s", service.runInput)
	}
}

func TestRunCommandProjectsJSONOutput(t *testing.T) {
	// #given
	service := &fakeRunService{
		runResult: runservice.Result{Run: readRun(), Output: json.RawMessage(`{"records":[]}`)},
	}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status,output", "run", "weekly.get_records", "--input", `{"week":"current"}`})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\"id\":\"run_read\",\"output\":{\"records\":[]},\"status\":\"succeeded\"}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCommandReadsInputFromPipe(t *testing.T) {
	// #given
	service := &fakeRunService{
		runResult: runservice.Result{Run: readRun(), Output: json.RawMessage(`{"records":[]}`)},
	}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(`{"week":"current"}`), &stdout, &stderr, false)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"run", "weekly.get_records"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "run_read succeeded weekly.get_records\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if string(service.runInput) != `{"week":"current"}` {
		t.Fatalf("Run() input = %s", service.runInput)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCommandRejectsTTYInputWithoutInputFlag(t *testing.T) {
	// #given
	service := &fakeRunService{}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(""), &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"run", "weekly.get_records"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input is required" {
		t.Fatalf("Execute() error = %v", err)
	}
	if service.runCalls != 0 {
		t.Fatalf("Run() calls = %d, want 0", service.runCalls)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCommandRejectsInputFlagWithNonEmptyPipe(t *testing.T) {
	// #given
	service := &fakeRunService{}
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(strings.NewReader(`{"other":true}`), &stdout, &stderr, false)
	f := &cmdutil.Factory{IO: ioStreams, RunService: service}
	root := NewRoot(f)
	root.SetArgs([]string{"run", "weekly.get_records", "--input", `{"week":"current"}`})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --input cannot be combined with non-empty stdin" {
		t.Fatalf("Execute() error = %v", err)
	}
	if service.runCalls != 0 {
		t.Fatalf("Run() calls = %d, want 0", service.runCalls)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

type fakeRunService struct {
	runResult     runservice.Result
	approveResult runservice.Result
	rejectResult  domain.Run

	runCalls    int
	runToolName string
	runInput    json.RawMessage
	approveID   string
	rejectID    string
}

func (s *fakeRunService) Run(_ context.Context, toolName string, input json.RawMessage) (runservice.Result, error) {
	s.runCalls++
	s.runToolName = toolName
	s.runInput = append(json.RawMessage(nil), input...)
	return s.runResult, nil
}

func (s *fakeRunService) Approve(_ context.Context, runID string) (runservice.Result, error) {
	s.approveID = runID
	return s.approveResult, nil
}

func (s *fakeRunService) Reject(_ context.Context, runID string) (domain.Run, error) {
	s.rejectID = runID
	return s.rejectResult, nil
}

func readRun() domain.Run {
	return domain.Run{
		ID:          "run_read",
		ToolName:    "weekly.get_records",
		Effect:      domain.EffectRead,
		Status:      domain.StatusSucceeded,
		RequestedAt: testRunTime(),
	}
}

func testRunTime() time.Time {
	return time.Date(2026, 6, 5, 12, 1, 0, 123456789, time.UTC)
}
