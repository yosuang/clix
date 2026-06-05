package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

func TestCheckCommandPrintsPrimaryResultToStdout(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"check"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok newline", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestCheckCommandChecksRunStoreWhenConfigured(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	runStore := &recordingRunStore{}
	f := &cmdutil.Factory{IO: io, RunStore: runStore}
	root := NewRoot(f)
	root.SetArgs([]string{"check"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !runStore.listCalled {
		t.Fatal("run store was not checked")
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok newline", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestUnknownCommandReturnsUsageErrorWithoutCobraSuggestions(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"chek"})

	// #when
	err := root.Execute()

	// #then
	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
	if got := err.Error(); got != "USAGE_ERROR: unknown command \"chek\"" {
		t.Fatalf("error = %q", got)
	}
}

func TestCheckExtraArgReturnsUsageErrorWithoutPrinting(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams}
	root := NewRoot(f)
	root.SetArgs([]string{"check", "extra"})

	// #when
	err := root.Execute()

	// #then
	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("Execute() error = %T %q, want protocol error", err, err)
	}
	if perr.Code != protocol.UsageError {
		t.Fatalf("error code = %q, want %q", perr.Code, protocol.UsageError)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestRootUsesInjectedStdin(t *testing.T) {
	// #given
	stdin := strings.NewReader("from injected stdin")
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(stdin, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: ioStreams}

	// #when
	root := NewRoot(f)
	reader := root.InOrStdin()

	// #then
	if reader != stdin {
		t.Fatalf("InOrStdin() = %T, want injected stdin reader", reader)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "from injected stdin" {
		t.Fatalf("stdin = %q, want injected content", string(got))
	}
}

func TestRootDisablesCobraSuggestions(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}

	// #when
	root := NewRoot(f)

	// #then
	if !root.DisableSuggestions {
		t.Fatal("DisableSuggestions = false, want true")
	}
}

func TestRootStoresJSONFieldsInFactory(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,status", "check"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"id", "status"}
	if !reflect.DeepEqual(f.Output.JSONFields, want) {
		t.Fatalf("JSONFields = %#v, want %#v", f.Output.JSONFields, want)
	}
}

type recordingRunStore struct {
	listCalled bool
}

func (s *recordingRunStore) InsertRun(context.Context, domain.Run) error {
	panic("InsertRun should not be called")
}

func (s *recordingRunStore) GetRun(context.Context, string) (domain.Run, error) {
	panic("GetRun should not be called")
}

func (s *recordingRunStore) ListRuns(context.Context, *domain.Status) ([]domain.Run, error) {
	s.listCalled = true
	return nil, nil
}

func (s *recordingRunStore) MarkPending(context.Context, string) error {
	panic("MarkPending should not be called")
}

func (s *recordingRunStore) ClaimPendingRun(context.Context, string, time.Time) (domain.Run, error) {
	panic("ClaimPendingRun should not be called")
}

func (s *recordingRunStore) MarkSucceeded(context.Context, string, time.Time) error {
	panic("MarkSucceeded should not be called")
}

func (s *recordingRunStore) MarkFailed(context.Context, string, time.Time, protocol.Code, string) error {
	panic("MarkFailed should not be called")
}

func (s *recordingRunStore) MarkRejected(context.Context, string, time.Time) error {
	panic("MarkRejected should not be called")
}

func TestRootRejectsEmptyJSONField(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "id,,status", "check"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: empty --json field" {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestRootRejectsReservedJQ(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"--jq", ".id", "check"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --jq is reserved for future use" {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestRootRejectsReservedJQWhenValueIsEmpty(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	root.SetArgs([]string{"--jq", "", "check"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --jq is reserved for future use" {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestRootOutputOptionsRunBeforeChildPersistentPreRun(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io}
	root := NewRoot(f)
	childHookRan := false
	root.AddCommand(&cobra.Command{
		Use:  "child",
		Args: usageArgs(cobra.NoArgs),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			childHookRan = true
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	})
	root.SetArgs([]string{"--json", "id,status", "child"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !childHookRan {
		t.Fatal("child persistent pre-run did not run")
	}
	want := []string{"id", "status"}
	if !reflect.DeepEqual(f.Output.JSONFields, want) {
		t.Fatalf("JSONFields = %#v, want %#v", f.Output.JSONFields, want)
	}
}
