package cmd

import (
	"bytes"
	"testing"

	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/iostreams"
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
