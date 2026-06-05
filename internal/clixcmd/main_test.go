package clixcmd

import (
	"bytes"
	"testing"

	"github.com/yosuang/clix/internal/iostreams"
)

func TestRunMapsUsageErrorToExitCodeTwo(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"check", "extra"})

	// #then
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "USAGE_ERROR: unknown command \"extra\" for \"clix check\"\n" {
		t.Fatalf("stderr = %q, want usage error", stderr.String())
	}
}

func TestRunRendersJSONErrorWhenJSONRequested(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"--json", "id", "check", "extra"})

	// #then
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	want := "{\"code\":\"USAGE_ERROR\",\"message\":\"unknown command \\\"extra\\\" for \\\"clix check\\\"\",\"ok\":false}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want JSON error %q", stderr.String(), want)
	}
}

func TestRunRendersJSONErrorForInvalidJSONFields(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"--json", "id,,status", "check"})

	// #then
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	want := "{\"code\":\"USAGE_ERROR\",\"message\":\"empty --json field\",\"ok\":false}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want JSON error %q", stderr.String(), want)
	}
}
