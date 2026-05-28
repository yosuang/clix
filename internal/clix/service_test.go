package clix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestWeeklyAcceptanceScenario(t *testing.T) {
	t.Setenv("WORK_API_TOKEN", "test-token")
	server, submitCount := newWeeklyServer(t)
	defer server.Close()
	manifestPath := writeWeeklyManifest(t, t.TempDir(), server.URL, "Submit a weekly report.")
	dbPath := filepath.Join(t.TempDir(), "clix.db")
	service, appErr := newService(manifestPath, dbPath, nil, true)
	requireNoAppErr(t, appErr)
	defer service.Close()

	//#given an agent needs to understand and run the weekly tools
	tools := service.listTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "weekly.get_records" || tools[0].Effect != "read" || tools[0].Adapter != "http" {
		t.Fatalf("unexpected read tool summary: %#v", tools[0])
	}
	detail, appErr := service.getTool("weekly.get_records")
	requireNoAppErr(t, appErr)
	if detail.InputSchema["type"] != "object" || detail.OutputSchema["type"] != "object" {
		t.Fatalf("tool schemas were not loaded: %#v", detail)
	}

	//#when a read action runs
	readResult, appErr := service.runTool(context.Background(), "weekly.get_records", `{"week":"current"}`)
	requireNoAppErr(t, appErr)

	//#then it executes immediately and returns JSON output
	if readResult.Status != StatusSucceeded {
		t.Fatalf("expected read run to succeed, got %s", readResult.Status)
	}
	readOutput, ok := readResult.Output.(map[string]any)
	if !ok || readOutput["week"] != "current" {
		t.Fatalf("unexpected read output: %#v", readResult.Output)
	}

	//#when a write action is requested
	writeResult, appErr := service.runTool(context.Background(), "weekly.submit_report", `{"week":"current","content":"done"}`)
	requireNoAppErr(t, appErr)

	//#then it is pending and the adapter has not been executed
	if writeResult.Status != StatusPendingApproval {
		t.Fatalf("expected pending write run, got %s", writeResult.Status)
	}
	if atomic.LoadInt32(submitCount) != 0 {
		t.Fatalf("write adapter executed before approval")
	}

	//#when the pending run is approved
	approved, appErr := service.approveRun(context.Background(), writeResult.ID)
	requireNoAppErr(t, appErr)

	//#then it executes once and stores terminal run metadata
	if approved.Status != StatusSucceeded {
		t.Fatalf("expected approved run to succeed, got %s", approved.Status)
	}
	if atomic.LoadInt32(submitCount) != 1 {
		t.Fatalf("expected exactly one submit call, got %d", atomic.LoadInt32(submitCount))
	}
	_, appErr = service.approveRun(context.Background(), writeResult.ID)
	if appErr == nil || appErr.Code != CodeApprovalError {
		t.Fatalf("expected duplicate approval to fail with %s, got %#v", CodeApprovalError, appErr)
	}
	if atomic.LoadInt32(submitCount) != 1 {
		t.Fatalf("duplicate approval executed adapter")
	}
	stored, appErr := service.getRun(writeResult.ID)
	requireNoAppErr(t, appErr)
	if stored.Status != StatusSucceeded || stored.ExitCode == nil || *stored.ExitCode != 0 {
		t.Fatalf("unexpected stored run: %#v", stored)
	}
}

func TestApproveRejectsPendingRunWhenManifestChanged(t *testing.T) {
	t.Setenv("WORK_API_TOKEN", "test-token")
	server, submitCount := newWeeklyServer(t)
	defer server.Close()
	dir := t.TempDir()
	manifestPath := writeWeeklyManifest(t, dir, server.URL, "Submit a weekly report.")
	dbPath := filepath.Join(t.TempDir(), "clix.db")

	//#given a pending write run stores the original tool fingerprint
	service, appErr := newService(manifestPath, dbPath, nil, true)
	requireNoAppErr(t, appErr)
	pending, appErr := service.runTool(context.Background(), "weekly.submit_report", `{"week":"current","content":"done"}`)
	requireNoAppErr(t, appErr)
	if pending.Status != StatusPendingApproval {
		t.Fatalf("expected pending run, got %s", pending.Status)
	}
	requireNoError(t, service.Close())

	//#when the tool definition changes before approval
	writeWeeklyManifest(t, dir, server.URL, "Submit a changed weekly report.")
	changedService, appErr := newService(manifestPath, dbPath, nil, true)
	requireNoAppErr(t, appErr)
	defer changedService.Close()
	result, appErr := changedService.approveRun(context.Background(), pending.ID)

	//#then approval is rejected without executing the adapter
	if appErr == nil || appErr.Code != CodeManifestChanged {
		t.Fatalf("expected %s, got result=%#v err=%#v", CodeManifestChanged, result, appErr)
	}
	if result.Status != StatusRejected {
		t.Fatalf("expected rejected status, got %#v", result)
	}
	if atomic.LoadInt32(submitCount) != 0 {
		t.Fatalf("changed manifest approval executed adapter")
	}
	stored, appErr := changedService.getRun(pending.ID)
	requireNoAppErr(t, appErr)
	if stored.Status != StatusRejected || stored.ErrorCode == nil || *stored.ErrorCode != CodeManifestChanged {
		t.Fatalf("unexpected stored rejection: %#v", stored)
	}
}

func TestCLIJSONProjectionAndJQ(t *testing.T) {
	t.Setenv("WORK_API_TOKEN", "test-token")
	server, _ := newWeeklyServer(t)
	defer server.Close()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	manifestPath := filepath.Join(home, ".config", "clix", "manifest.yaml")
	requireNoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o700))
	writeWeeklyManifestAt(t, manifestPath, server.URL, "Submit a weekly report.")

	//#given the CLI uses the user-global manifest and database paths
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	//#when JSON projection is combined with jq
	code := Run([]string{
		"tools", "list",
		"--json", "name,effect,adapter",
		"--jq", `.[] | select(.effect == "write")`,
	}, &stdout, &stderr)

	//#then only the selected write tool fields are returned
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var selected map[string]any
	requireNoError(t, json.Unmarshal(stdout.Bytes(), &selected))
	if selected["name"] != "weekly.submit_report" || selected["effect"] != "write" {
		t.Fatalf("unexpected projected jq output: %#v", selected)
	}

	//#when persistent output flags are placed before subcommands
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{
		"--json", "name,effect",
		"tools", "get", "weekly.get_records",
	}, &stdout, &stderr)

	//#then Cobra/Pflag parses them as global flags
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var globalFlagResult map[string]any
	requireNoError(t, json.Unmarshal(stdout.Bytes(), &globalFlagResult))
	if globalFlagResult["name"] != "weekly.get_records" || globalFlagResult["effect"] != "read" {
		t.Fatalf("unexpected global flag output: %#v", globalFlagResult)
	}

	//#when jq is used without an explicit json projection
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{
		"tools", "list",
		"--jq", `[.[] | select(.effect == "read")][0].name`,
	}, &stdout, &stderr)

	//#then jq implies JSON output over the full command result
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var jqOnly string
	requireNoError(t, json.Unmarshal(stdout.Bytes(), &jqOnly))
	if jqOnly != "weekly.get_records" {
		t.Fatalf("unexpected jq-only output: %q", jqOnly)
	}

	//#when a read action is executed through the CLI in JSON mode
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{
		"run", "weekly.get_records",
		"--input", `{"week":"current"}`,
		"--json", "id,status,output",
	}, &stdout, &stderr)

	//#then run metadata and adapter output are selectable
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var run map[string]any
	requireNoError(t, json.Unmarshal(stdout.Bytes(), &run))
	if run["status"] != StatusSucceeded {
		t.Fatalf("unexpected run output: %#v", run)
	}
	if _, ok := run["id"].(string); !ok {
		t.Fatalf("run id was not returned: %#v", run)
	}
	output, ok := run["output"].(map[string]any)
	if !ok || output["week"] != "current" {
		t.Fatalf("adapter output was not returned: %#v", run)
	}

	//#when a command fails in JSON mode
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{
		"tools", "get", "weekly.missing",
		"--json", "name",
	}, &stdout, &stderr)

	//#then the failure is a stable JSON error object
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing tool")
	}
	var failure map[string]any
	requireNoError(t, json.Unmarshal(stdout.Bytes(), &failure))
	if failure["ok"] != false || failure["code"] != CodeToolNotFound {
		t.Fatalf("unexpected JSON failure: %#v stderr=%s", failure, stderr.String())
	}
}

func newWeeklyServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var submitCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/records":
			if r.Method != http.MethodGet {
				t.Errorf("expected GET /records, got %s", r.Method)
			}
			week := r.URL.Query().Get("week")
			fmt.Fprintf(w, `{"week":%q,"records":[{"id":"rec_1"}]}`, week)
		case "/reports":
			atomic.AddInt32(&submitCount, 1)
			if r.Method != http.MethodPost {
				t.Errorf("expected POST /reports, got %s", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("unexpected Authorization header: %q", r.Header.Get("Authorization"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode report body: %v", err)
			}
			fmt.Fprintf(w, `{"submitted":true,"week":%q}`, body["week"])
		default:
			http.NotFound(w, r)
		}
	}))
	return server, &submitCount
}

func writeWeeklyManifest(t *testing.T, dir, baseURL, submitDescription string) string {
	t.Helper()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	writeWeeklyManifestAt(t, manifestPath, baseURL, submitDescription)
	return manifestPath
}

func writeWeeklyManifestAt(t *testing.T, manifestPath, baseURL, submitDescription string) {
	t.Helper()
	requireNoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o700))
	content := fmt.Sprintf(`version: 1

tools:
  weekly.get_records:
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
      url: "%s/records?week=${input.week}"
  weekly.submit_report:
    description: %s
    adapter: http
    effect: write
    secrets:
      - WORK_API_TOKEN
    input_schema:
      type: object
      additionalProperties: false
      required: [week, content]
      properties:
        week:
          type: string
        content:
          type: string
    output_schema:
      type: object
    http:
      method: POST
      url: "%s/reports"
      headers:
        Authorization: "Bearer ${secrets.WORK_API_TOKEN}"
      json_body:
        week: "${input.week}"
        content: "${input.content}"
`, baseURL, submitDescription, baseURL)
	requireNoError(t, os.WriteFile(manifestPath, []byte(content), 0o600))
}

func requireNoAppErr(t *testing.T, appErr *AppError) {
	t.Helper()
	if appErr != nil {
		t.Fatalf("unexpected app error: %v", appErr)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
