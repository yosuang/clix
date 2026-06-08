package acceptance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWeeklyReportAcceptanceScenario(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)

	// #when
	list := runClix(t, home, nil, "tools", "list", "--json", "name,effect,adapter,description")
	get := runClix(t, home, nil, "tools", "get", "weekly.get_records", "--json", "name,effect,input_schema,output_schema")
	read := runClix(t, home, nil, "run", "weekly.get_records", "--input", `{"week":"current"}`, "--json", "id,status,output")
	write := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")

	// #then
	list.wantStdoutContains(`"weekly.get_records"`)
	get.wantStdoutContains(`"input_schema"`)
	read.wantStdoutContains(`"status":"succeeded"`)
	write.wantStdoutContains(`"status":"pending_approval"`)
	write.wantStderr("")
}

func TestReservedJQFails(t *testing.T) {
	// #given
	home := t.TempDir()

	// #when
	got := runClixAllowFailure(t, home, nil, "check", "--jq", ".id")

	// #then
	got.wantExitCode(2)
	got.wantStdout("")
	got.wantStderrContains("USAGE_ERROR: --jq is reserved for future use")
}

func TestJSONFailureWritesOnlyErrorObjectToStderr(t *testing.T) {
	// #given
	home := t.TempDir()

	// #when
	got := runClixAllowFailure(t, home, nil, "tools", "get", "missing.tool", "--json", "name")

	// #then
	got.wantExitCode(1)
	got.wantStdout("")
	got.wantStderrJSONError("TOOL_NOT_FOUND", `tool "missing.tool" not found`)
}

func TestDecodeOnlyJSONErrorRejectsDiagnosticsAndExtraFields(t *testing.T) {
	cases := map[string]string{
		"prefix text":      `diagnostic {"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found"}` + "\n",
		"suffix text":      `{"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found"}` + "\nextra\n",
		"multiple values":  `{"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found"} {"ok":false}` + "\n",
		"extra field":      `{"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found","detail":"x"}` + "\n",
		"malformed json":   `{"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found"` + "\n",
		"non-object json":  `["ok","code","message"]` + "\n",
		"missing message":  `{"ok":false,"code":"TOOL_NOT_FOUND"}` + "\n",
		"non-false ok":     `{"ok":true,"code":"TOOL_NOT_FOUND","message":"tool not found"}` + "\n",
		"non-string code":  `{"ok":false,"code":404,"message":"tool not found"}` + "\n",
		"non-string error": `{"ok":false,"code":"TOOL_NOT_FOUND","message":404}` + "\n",
	}

	for name, stderr := range cases {
		t.Run(name, func(t *testing.T) {
			// #given
			input := []byte(stderr)

			// #when
			_, err := decodeOnlyJSONError(input)

			// #then
			if err == nil {
				t.Fatal("decodeOnlyJSONError() error = nil, want strict JSON error rejection")
			}
		})
	}
}

func TestDecodeOnlyJSONErrorAcceptsExactlyOneErrorObject(t *testing.T) {
	// #given
	stderr := []byte(`{"ok":false,"code":"TOOL_NOT_FOUND","message":"tool not found"}` + "\n")

	// #when
	got, err := decodeOnlyJSONError(stderr)

	// #then
	if err != nil {
		t.Fatalf("decodeOnlyJSONError() error = %v", err)
	}
	if got.Code != "TOOL_NOT_FOUND" || got.Message != "tool not found" {
		t.Fatalf("decoded error = %#v", got)
	}
}

func TestWriteActionCreatesPendingRun(t *testing.T) {
	// #given
	home := t.TempDir()
	weekly := writeWeeklyTools(t, home)

	// #when
	got := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")

	// #then
	got.wantStdoutContains(`"status":"pending_approval"`)
	got.wantStdoutContains(`"tool_name":"weekly.submit_report"`)
	got.wantStderr("")
	if weekly.reportCallCount() != 0 {
		t.Fatalf("report calls = %d, want 0 before approval", weekly.reportCallCount())
	}
}

func TestApprovePendingRunSucceedsAndReturnsOutput(t *testing.T) {
	// #given
	home := t.TempDir()
	weekly := writeWeeklyTools(t, home)
	pending := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")
	runID := pending.stdoutFieldString("id")

	// #when
	got := runClix(t, home, nil, "approve", runID, "--json", "id,status,output")

	// #then
	got.wantStdoutContains(`"id":"` + runID + `"`)
	got.wantStdoutContains(`"status":"succeeded"`)
	got.wantStdoutContains(`"accepted":true`)
	got.wantStderr("")
	if weekly.reportCallCount() != 1 {
		t.Fatalf("report calls = %d, want 1", weekly.reportCallCount())
	}
}

func TestSecondApproveOfSameRunFails(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	pending := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")
	runID := pending.stdoutFieldString("id")
	runClix(t, home, nil, "approve", runID, "--json", "id,status,output")

	// #when
	got := runClixAllowFailure(t, home, nil, "approve", runID)

	// #then
	got.wantExitCode(1)
	got.wantStdout("")
	got.wantStderrContains("APPROVAL_ERROR")
}

func TestChangedWriteToolFingerprintFailsApprove(t *testing.T) {
	// #given
	home := t.TempDir()
	weekly := writeWeeklyTools(t, home)
	pending := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")
	runID := pending.stdoutFieldString("id")
	callsBefore := weekly.reportCallCount()
	weekly.writeSubmitTool(t, "Submit a changed weekly report.")

	// #when
	got := runClixAllowFailure(t, home, nil, "approve", runID)

	// #then
	got.wantExitCode(1)
	got.wantStdout("")
	got.wantStderrContains("TOOL_CHANGED")
	if weekly.reportCallCount() != callsBefore {
		t.Fatalf("report calls = %d, want unchanged %d", weekly.reportCallCount(), callsBefore)
	}
}

func TestRejectPendingRunMovesItToRejected(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	pending := runClix(t, home, nil, "run", "weekly.submit_report", "--input", `{"week":"current","content":"done"}`, "--json", "id,status,tool_name")
	runID := pending.stdoutFieldString("id")

	// #when
	got := runClix(t, home, nil, "reject", runID, "--json", "id,status")
	stored := runClix(t, home, nil, "runs", "get", runID, "--json", "id,status")

	// #then
	got.wantStdoutContains(`"status":"rejected"`)
	stored.wantStdoutContains(`"status":"rejected"`)
	got.wantStderr("")
}

func TestRunsGetReturnsErrorCodeAndMessageForFailedRuns(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	failed := runClixAllowFailure(t, home, nil, "run", "weekly.get_records", "--input", `{"week":"fail"}`, "--json", "id,status")
	failed.wantExitCode(1)
	failed.wantStdout("")
	failed.wantStderrContains(`"code":"ADAPTER_ERROR"`)
	list := runClix(t, home, nil, "runs", "list", "--status", "failed", "--json", "id,status,error_code,error_message")
	runID := list.stdoutFirstListFieldString("id")

	// #when
	got := runClix(t, home, nil, "runs", "get", runID, "--json", "id,status,error_code,error_message")

	// #then
	got.wantStdoutContains(`"status":"failed"`)
	got.wantStdoutContains(`"error_code":"ADAPTER_ERROR"`)
	got.wantStdoutContains(`"error_message":"HTTP request failed with status 500"`)
}

func TestRunReadsOneJSONObjectFromPipedStdin(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	stdin := strings.NewReader(`{"week":"current"}`)

	// #when
	got := runClix(t, home, stdin, "run", "weekly.get_records", "--json", "id,status,output")

	// #then
	got.wantStdoutContains(`"status":"succeeded"`)
	got.wantStdoutContains(`"records"`)
	got.wantStderr("")
}

func TestRunRejectsMultipleJSONValuesFromPipedStdin(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	stdin := strings.NewReader(`{"week":"current"} {"extra":true}`)

	// #when
	got := runClixAllowFailure(t, home, stdin, "run", "weekly.get_records")

	// #then
	got.wantExitCode(1)
	got.wantStdout("")
	got.wantStderrContains("VALIDATION_ERROR: input must contain exactly one JSON object")
}

func TestRunRejectsInputFlagCombinedWithNonEmptyPipedStdin(t *testing.T) {
	// #given
	home := t.TempDir()
	writeWeeklyTools(t, home)
	stdin := strings.NewReader(`{"week":"current"}`)

	// #when
	got := runClixAllowFailure(t, home, stdin, "run", "weekly.get_records", "--input", `{"week":"current"}`)

	// #then
	got.wantExitCode(2)
	got.wantStdout("")
	got.wantStderrContains("USAGE_ERROR: --input cannot be combined with non-empty stdin")
}

type result struct {
	t      *testing.T
	stdout bytes.Buffer
	stderr bytes.Buffer
	code   int
}

func runClix(t *testing.T, home string, stdin io.Reader, args ...string) *result {
	t.Helper()
	got := runClixAllowFailure(t, home, stdin, args...)
	if got.code != 0 {
		t.Fatalf("clix %s exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), got.code, got.stdout.String(), got.stderr.String())
	}
	return got
}

func runClixAllowFailure(t *testing.T, home string, stdin io.Reader, args ...string) *result {
	t.Helper()
	got := &result{t: t}
	cmdArgs := append([]string{"run", "./cmd/clix"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = repoRoot(t)
	cmd.Env = subprocessEnv(t, home)
	cmd.Stdout = &got.stdout
	cmd.Stderr = &got.stderr
	if stdin != nil {
		cmd.Stdin = stdin
	}
	err := cmd.Run()
	if err == nil {
		return got
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		got.code = exitErr.ExitCode()
		got.normalizeGoRunExitStatus()
		return got
	}
	t.Fatalf("clix %s failed to start: %v", strings.Join(args, " "), err)
	return got
}

func (r *result) wantExitCode(want int) {
	r.t.Helper()
	if r.code != want {
		r.t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, want, r.stdout.String(), r.stderr.String())
	}
}

func (r *result) wantStdout(want string) {
	r.t.Helper()
	if got := r.stdout.String(); got != want {
		r.t.Fatalf("stdout = %q, want %q\nstderr:\n%s", got, want, r.stderr.String())
	}
}

func (r *result) wantStderr(want string) {
	r.t.Helper()
	if got := r.stderr.String(); got != want {
		r.t.Fatalf("stderr = %q, want %q\nstdout:\n%s", got, want, r.stdout.String())
	}
}

func (r *result) wantStdoutContains(want string) {
	r.t.Helper()
	if got := r.stdout.String(); !strings.Contains(got, want) {
		r.t.Fatalf("stdout = %q, want it to contain %q\nstderr:\n%s", got, want, r.stderr.String())
	}
}

func (r *result) wantStderrContains(want string) {
	r.t.Helper()
	if got := r.stderr.String(); !strings.Contains(got, want) {
		r.t.Fatalf("stderr = %q, want it to contain %q\nstdout:\n%s", got, want, r.stdout.String())
	}
}

func (r *result) wantStderrJSONError(wantCode string, wantMessage string) {
	r.t.Helper()
	got, err := decodeOnlyJSONError(r.stderr.Bytes())
	if err != nil {
		r.t.Fatalf("stderr JSON error decode = %v\nstderr:\n%s\nstdout:\n%s", err, r.stderr.String(), r.stdout.String())
	}
	if got.Code != wantCode || got.Message != wantMessage {
		r.t.Fatalf("stderr JSON error = %#v, want code %q message %q", got, wantCode, wantMessage)
	}
}

func (r *result) stdoutFieldString(field string) string {
	r.t.Helper()
	var object map[string]any
	if err := json.Unmarshal(r.stdout.Bytes(), &object); err != nil {
		r.t.Fatalf("stdout JSON object decode error = %v\nstdout:\n%s", err, r.stdout.String())
	}
	value, ok := object[field].(string)
	if !ok {
		r.t.Fatalf("stdout field %q = %#v, want string", field, object[field])
	}
	return value
}

func (r *result) stdoutFirstListFieldString(field string) string {
	r.t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(r.stdout.Bytes(), &list); err != nil {
		r.t.Fatalf("stdout JSON list decode error = %v\nstdout:\n%s", err, r.stdout.String())
	}
	if len(list) == 0 {
		r.t.Fatalf("stdout JSON list is empty")
	}
	value, ok := list[0][field].(string)
	if !ok {
		r.t.Fatalf("stdout first list field %q = %#v, want string", field, list[0][field])
	}
	return value
}

func (r *result) normalizeGoRunExitStatus() {
	raw := r.stderr.String()
	trimmed := strings.TrimRight(raw, "\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	lastLine := strings.TrimSpace(trimmed[lineStart:])
	codeText, ok := strings.CutPrefix(lastLine, "exit status ")
	if !ok {
		return
	}
	code, err := strconv.Atoi(codeText)
	if err != nil {
		return
	}
	r.code = code
	r.stderr.Reset()
	r.stderr.WriteString(raw[:lineStart])
}

type jsonErrorObject struct {
	Code    string
	Message string
}

func decodeOnlyJSONError(stderr []byte) (jsonErrorObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(stderr))
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return jsonErrorObject{}, fmt.Errorf("stderr must be one JSON object: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return jsonErrorObject{}, fmt.Errorf("stderr must contain exactly one JSON object")
	}
	if len(raw) != 3 {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error must contain exactly ok, code, and message fields")
	}

	okRaw, hasOK := raw["ok"]
	codeRaw, hasCode := raw["code"]
	messageRaw, hasMessage := raw["message"]
	if !hasOK || !hasCode || !hasMessage {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error must contain exactly ok, code, and message fields")
	}

	var ok bool
	if err := json.Unmarshal(okRaw, &ok); err != nil {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error ok must be false")
	}
	if ok {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error ok must be false")
	}

	var out jsonErrorObject
	if err := json.Unmarshal(codeRaw, &out.Code); err != nil {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error code must be a string")
	}
	if err := json.Unmarshal(messageRaw, &out.Message); err != nil {
		return jsonErrorObject{}, fmt.Errorf("stderr JSON error message must be a string")
	}
	return out, nil
}

type weeklyTools struct {
	server      *httptest.Server
	toolsDir    string
	submitPath  string
	reportCalls atomic.Int64
}

func writeWeeklyTools(t *testing.T, home string) *weeklyTools {
	t.Helper()
	weekly := &weeklyTools{
		toolsDir:   filepath.Join(home, ".config", "clix", "tools"),
		submitPath: filepath.Join(home, ".config", "clix", "tools", "weekly.submit_report.yaml"),
	}
	weekly.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/records":
			handleRecords(w, r)
		case "/reports":
			weekly.reportCalls.Add(1)
			handleReports(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(weekly.server.Close)

	if err := os.MkdirAll(weekly.toolsDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(weekly.toolsDir, "weekly.get_records.yaml"), readToolYAML(weekly.server.URL))
	weekly.writeSubmitTool(t, "Submit a weekly report.")
	return weekly
}

func (w *weeklyTools) writeSubmitTool(t *testing.T, description string) {
	t.Helper()
	writeFile(t, w.submitPath, submitToolYAML(w.server.URL, description))
}

func (w *weeklyTools) reportCallCount() int64 {
	return w.reportCalls.Load()
}

func handleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	week := r.URL.Query().Get("week")
	if week == "fail" {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"records":[{"week":%q,"status":"done"}]}`, week)
}

func handleReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Authorization") != "Bearer secret-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"accepted":true,"week":%q}`, body["week"])
}

func readToolYAML(serverURL string) string {
	return fmt.Sprintf(`version: 1
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
  url: "%s/records?week=${input.week}"
`, serverURL)
}

func submitToolYAML(serverURL string, description string) string {
	return fmt.Sprintf(`version: 1
name: weekly.submit_report
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
`, description, serverURL)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s does not contain go.mod: %v", root, err)
	}
	return root
}

func subprocessEnv(t *testing.T, home string) []string {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && isOverriddenEnv(name) {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "HOME="+home)
	env = append(env, "USERPROFILE="+home)
	env = append(env, "WORK_API_TOKEN=secret-token")
	env = append(env, currentGoToolEnv(t)...)
	return env
}

func isOverriddenEnv(name string) bool {
	switch runtime.GOOS {
	case "windows":
		return strings.EqualFold(name, "HOME") ||
			strings.EqualFold(name, "USERPROFILE") ||
			strings.EqualFold(name, "WORK_API_TOKEN") ||
			strings.EqualFold(name, "GOCACHE") ||
			strings.EqualFold(name, "GOMODCACHE") ||
			strings.EqualFold(name, "GOPATH")
	default:
		return name == "HOME" ||
			name == "USERPROFILE" ||
			name == "WORK_API_TOKEN" ||
			name == "GOCACHE" ||
			name == "GOMODCACHE" ||
			name == "GOPATH"
	}
}

var goToolEnv struct {
	once  sync.Once
	value []string
	err   error
}

func currentGoToolEnv(t *testing.T) []string {
	t.Helper()
	goToolEnv.once.Do(func() {
		cmd := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH")
		cmd.Dir = repoRoot(t)
		out, err := cmd.Output()
		if err != nil {
			goToolEnv.err = err
			return
		}
		lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
		if len(lines) != 3 {
			goToolEnv.err = fmt.Errorf("go env returned %d lines, want 3: %q", len(lines), out)
			return
		}
		goToolEnv.value = []string{
			"GOCACHE=" + strings.TrimSpace(lines[0]),
			"GOMODCACHE=" + strings.TrimSpace(lines[1]),
			"GOPATH=" + strings.TrimSpace(lines[2]),
		}
	})
	if goToolEnv.err != nil {
		t.Fatalf("go env for subprocess cache failed: %v", goToolEnv.err)
	}
	return append([]string(nil), goToolEnv.value...)
}
