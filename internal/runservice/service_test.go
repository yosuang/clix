package runservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yosuang/clix/internal/adapter"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func TestRunReadToolExecutesImmediately(t *testing.T) {
	// #given
	store := newMemoryStore()
	adapters := newMemoryAdapters(map[string]json.RawMessage{
		"http": []byte(`{"records":[]}`),
	})
	service := New(ServiceOptions{Store: store, Catalog: catalogWith(readTool()), Adapters: adapters, IDs: fixedIDs("run_read")})

	// #when
	result, err := service.Run(context.Background(), "weekly.get_records", []byte(`{"week":"current"}`))

	// #then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Run.Status != domain.StatusSucceeded {
		t.Fatalf("status = %s", result.Run.Status)
	}
	if string(result.Output) != `{"records":[]}` {
		t.Fatalf("output = %s", result.Output)
	}
}

func TestRunWriteToolStopsAtPendingApproval(t *testing.T) {
	// #given
	store := newMemoryStore()
	adapters := newMemoryAdapters(nil)
	service := New(ServiceOptions{Store: store, Catalog: catalogWith(writeTool()), Adapters: adapters, IDs: fixedIDs("run_write")})

	// #when
	result, err := service.Run(context.Background(), "weekly.submit_report", []byte(`{"week":"current","content":"done"}`))

	// #then
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Run.Status != domain.StatusPendingApproval {
		t.Fatalf("status = %s", result.Run.Status)
	}
	if adapters.Calls != 0 {
		t.Fatalf("adapter calls = %d, want 0", adapters.Calls)
	}
}

func TestApproveRejectsChangedToolBeforeAdapterExecution(t *testing.T) {
	// #given
	store := newMemoryStore()
	original := writeTool()
	service := New(ServiceOptions{Store: store, Catalog: catalogWith(original), Adapters: newMemoryAdapters(nil), IDs: fixedIDs("run_write")})
	created, err := service.Run(context.Background(), original.Name, []byte(`{"week":"current","content":"done"}`))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	changed := original
	changed.Fingerprint = "changed"
	adapters := newMemoryAdapters(map[string]json.RawMessage{"http": []byte(`{"ok":true}`)})
	service = New(ServiceOptions{Store: store, Catalog: catalogWith(changed), Adapters: adapters, IDs: fixedIDs("unused")})

	// #when
	_, err = service.Approve(context.Background(), created.Run.ID)

	// #then
	if err == nil || err.Error() != "TOOL_CHANGED: tool definition changed before approval" {
		t.Fatalf("Approve() error = %v", err)
	}
	if adapters.Calls != 0 {
		t.Fatalf("adapter calls = %d, want 0", adapters.Calls)
	}
}

func TestRunValidatesInputBeforeInsert(t *testing.T) {
	// #given
	store := newMemoryStore()
	catalog := catalogWith(readTool())
	catalog.validationErr = protocol.NewError(protocol.ValidationError, "input.week is required")
	service := New(ServiceOptions{Store: store, Catalog: catalog, Adapters: newMemoryAdapters(nil), IDs: fixedIDs("run_invalid")})

	// #when
	_, err := service.Run(context.Background(), "weekly.get_records", []byte(`{}`))

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input.week is required" {
		t.Fatalf("Run() error = %v", err)
	}
	if store.Inserts != 0 {
		t.Fatalf("inserts = %d, want 0", store.Inserts)
	}
}

func TestRunDoesNotPersistSecretFromAdapterError(t *testing.T) {
	// #given
	secretValue := "secret-token-in-url"
	store := newMemoryStore()
	tool := readTool()
	tool.Secrets = []string{"WORK_API_TOKEN"}
	tool.AdapterConfig = map[string]any{
		"method": "GET",
		"url":    "https://example.invalid/records?token=${secrets.WORK_API_TOKEN}",
	}
	client := &http.Client{Transport: runserviceRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("request failed for %s", req.URL.String())
	})}
	service := New(ServiceOptions{
		Store:   store,
		Catalog: catalogWith(tool),
		Adapters: adapter.NewRegistry(
			adapter.WithSecrets(map[string]string{"WORK_API_TOKEN": secretValue}),
			adapter.WithHTTPClient(client),
		),
		IDs: fixedIDs("run_secret"),
	})

	// #when
	result, err := service.Run(context.Background(), tool.Name, []byte(`{}`))

	// #then
	if err == nil {
		t.Fatal("Run() error = nil")
	}
	if result.Run.ErrorMessage == nil {
		t.Fatal("ErrorMessage = nil")
	}
	if strings.Contains(*result.Run.ErrorMessage, secretValue) {
		t.Fatalf("stored error message leaked secret: %q", *result.Run.ErrorMessage)
	}
}

type memoryStore struct {
	Runs    map[string]domain.Run
	Inserts int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{Runs: map[string]domain.Run{}}
}

func (s *memoryStore) InsertRun(_ context.Context, run domain.Run) error {
	s.Inserts++
	s.Runs[run.ID] = run
	return nil
}

func (s *memoryStore) GetRun(_ context.Context, id string) (domain.Run, error) {
	run, ok := s.Runs[id]
	if !ok {
		return domain.Run{}, protocol.NewError(protocol.RunNotFound, "run not found")
	}
	return run, nil
}

func (s *memoryStore) ClaimPendingRun(_ context.Context, id string, startedAt time.Time) (domain.Run, error) {
	run, ok := s.Runs[id]
	if !ok {
		return domain.Run{}, protocol.NewError(protocol.RunNotFound, "run not found")
	}
	if run.Status != domain.StatusPendingApproval || run.Effect != domain.EffectWrite {
		return domain.Run{}, protocol.NewError(protocol.ApprovalError, "run is not pending approval")
	}
	run.Status = domain.StatusRunning
	run.ApprovedAt = &startedAt
	run.StartedAt = &startedAt
	s.Runs[id] = run
	return run, nil
}

func (s *memoryStore) MarkSucceeded(_ context.Context, id string, finishedAt time.Time) error {
	run, ok := s.Runs[id]
	if !ok {
		return protocol.NewError(protocol.RunNotFound, "run not found")
	}
	exitCode := 0
	run.Status = domain.StatusSucceeded
	run.FinishedAt = &finishedAt
	run.ExitCode = &exitCode
	run.ErrorCode = nil
	run.ErrorMessage = nil
	s.Runs[id] = run
	return nil
}

func (s *memoryStore) MarkFailed(_ context.Context, id string, finishedAt time.Time, code protocol.Code, message string) error {
	run, ok := s.Runs[id]
	if !ok {
		return protocol.NewError(protocol.RunNotFound, "run not found")
	}
	exitCode := 1
	codeText := string(code)
	run.Status = domain.StatusFailed
	run.FinishedAt = &finishedAt
	run.ExitCode = &exitCode
	run.ErrorCode = &codeText
	run.ErrorMessage = &message
	s.Runs[id] = run
	return nil
}

func (s *memoryStore) MarkRejected(_ context.Context, id string, finishedAt time.Time) error {
	run, ok := s.Runs[id]
	if !ok {
		return protocol.NewError(protocol.RunNotFound, "run not found")
	}
	if run.Status != domain.StatusPendingApproval || run.Effect != domain.EffectWrite {
		return protocol.NewError(protocol.ApprovalError, "run is not pending approval")
	}
	run.Status = domain.StatusRejected
	run.FinishedAt = &finishedAt
	s.Runs[id] = run
	return nil
}

type memoryCatalog struct {
	tools         map[string]domain.Tool
	validationErr error
}

func catalogWith(tools ...domain.Tool) *memoryCatalog {
	catalog := &memoryCatalog{tools: map[string]domain.Tool{}}
	for _, tool := range tools {
		catalog.tools[tool.Name] = tool
	}
	return catalog
}

func (c *memoryCatalog) Get(name string) (domain.Tool, bool) {
	tool, ok := c.tools[name]
	return tool, ok
}

func (c *memoryCatalog) ValidateInput(string, json.RawMessage) error {
	return c.validationErr
}

type memoryAdapters struct {
	Outputs map[string]json.RawMessage
	Calls   int
	Err     error
}

func newMemoryAdapters(outputs map[string]json.RawMessage) *memoryAdapters {
	return &memoryAdapters{Outputs: outputs}
}

func (a *memoryAdapters) Execute(_ context.Context, tool domain.Tool, _ json.RawMessage) (json.RawMessage, error) {
	a.Calls++
	if a.Err != nil {
		return nil, a.Err
	}
	return a.Outputs[tool.Adapter], nil
}

type fixedIDs string

func (ids fixedIDs) NewRunID() (string, error) {
	return string(ids), nil
}

func readTool() domain.Tool {
	return domain.Tool{
		Name:        "weekly.get_records",
		Adapter:     "http",
		Effect:      domain.EffectRead,
		SourcePath:  "tools/weekly.get_records.yaml",
		Fingerprint: "fingerprint-read",
	}
}

func writeTool() domain.Tool {
	return domain.Tool{
		Name:        "weekly.submit_report",
		Adapter:     "http",
		Effect:      domain.EffectWrite,
		SourcePath:  "tools/weekly.submit_report.yaml",
		Fingerprint: "fingerprint-write",
	}
}

type runserviceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f runserviceRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
