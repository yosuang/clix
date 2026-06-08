package clixcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
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

func TestRunHelpPrintsUsageWithoutCrashing(t *testing.T) {
	// #given
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunHelpSubprocess$")
	cmd.Env = append(os.Environ(), "CLIX_HELP_SUBPROCESS=1")

	// #when
	output, err := cmd.CombinedOutput()

	// #then
	if err != nil {
		t.Fatalf("help subprocess failed: %v\n%s", err, output)
	}
	if got := string(output); !strings.Contains(got, "Usage:\n  clix") {
		t.Fatalf("help output = %q, want root usage", got)
	}
}

func TestRunHelpSubprocess(t *testing.T) {
	if os.Getenv("CLIX_HELP_SUBPROCESS") != "1" {
		return
	}

	// #given
	debug.SetMaxStack(1 << 20)
	ioStreams := iostreams.TestIO(nil, os.Stdout, os.Stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"--help"})

	// #then
	os.Exit(exitCode)
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
	want := "{\"ok\":false,\"code\":\"USAGE_ERROR\",\"message\":\"unknown command \\\"extra\\\" for \\\"clix check\\\"\"}\n"
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
	want := "{\"ok\":false,\"code\":\"USAGE_ERROR\",\"message\":\"empty --json field\"}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want JSON error %q", stderr.String(), want)
	}
}

func TestRunRendersJSONErrorWhenJSONValueMissing(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"--json"})

	// #then
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.HasPrefix(stderr.String(), `{"ok":false,"code":"USAGE_ERROR","message":`) {
		t.Fatalf("stderr = %q, want JSON usage error", stderr.String())
	}
}

func TestRunLoadsUserCatalogForToolsList(t *testing.T) {
	// #given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeUserTool(t, home, userToolYAML())
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"tools", "list"})

	// #then
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	want := "weekly.get_records read http - Get work records for a given week.\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckRejectsMalformedUserCatalog(t *testing.T) {
	// #given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeUserTool(t, home, strings.Replace(userToolYAML(), "  method: GET\n", "", 1))
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"check"})

	// #then
	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	want := "TOOL_CATALOG_ERROR: weekly.get_records http.method is required\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunCheckCreatesUserRunStore(t *testing.T) {
	// #given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	exitCode := Run(ioStreams, []string{"check"})

	// #then
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok newline", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "clix", "clix.db")); err != nil {
		t.Fatalf("run store was not created: %v", err)
	}
}

func TestNewFactoryWiresRunServiceAndStore(t *testing.T) {
	// #given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeUserTool(t, home, userToolYAML())
	var stdout, stderr bytes.Buffer
	ioStreams := iostreams.TestIO(nil, &stdout, &stderr, true)

	// #when
	f, cleanup, err := newFactory(ioStreams)
	if err != nil {
		t.Fatalf("newFactory() error = %v", err)
	}
	defer cleanup()

	// #then
	if f.RunService == nil {
		t.Fatal("RunService = nil")
	}
	if f.RunStore == nil {
		t.Fatal("RunStore = nil")
	}
	loaded, err := f.CatalogLoader.Load()
	if err != nil {
		t.Fatalf("CatalogLoader.Load() error = %v", err)
	}
	if _, ok := loaded.Get("weekly.get_records"); !ok {
		t.Fatal("catalog is missing weekly.get_records")
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "clix", "clix.db")); err != nil {
		t.Fatalf("run store was not created: %v", err)
	}
}

func writeUserTool(t *testing.T, home string, body string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "clix", "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "weekly.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func userToolYAML() string {
	return `version: 1
name: weekly.get_records
description: Get work records for a given week.
adapter: http
effect: read
input_schema:
  type: object
  additionalProperties: false
  required: [week]
  properties:
    week:
      type: string
output_schema:
  type: object
http:
  method: GET
  url: "https://example.com/api/records?week=${input.week}"
`
}
