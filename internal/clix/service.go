package clix

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

type Service struct {
	manifest *Manifest
	store    *Store
	adapter  *HTTPAdapter
}

type CheckResult struct {
	OK           bool   `json:"ok"`
	ManifestPath string `json:"manifest_path"`
	DatabasePath string `json:"database_path"`
	ToolCount    int    `json:"tool_count"`
}

type RunResult struct {
	ID              string  `json:"id"`
	ToolName        string  `json:"tool_name"`
	Effect          string  `json:"effect"`
	ToolFingerprint string  `json:"tool_fingerprint"`
	Status          string  `json:"status"`
	RequestedAt     string  `json:"requested_at"`
	ApprovedAt      *string `json:"approved_at"`
	StartedAt       *string `json:"started_at"`
	FinishedAt      *string `json:"finished_at"`
	ExitCode        *int    `json:"exit_code"`
	ErrorCode       *string `json:"error_code"`
	ErrorMessage    *string `json:"error_message"`
	Output          any     `json:"output"`
}

func newDefaultService(requireManifest bool) (*Service, string, string, *AppError) {
	manifestPath, appErr := defaultManifestPath()
	if appErr != nil {
		return nil, "", "", appErr
	}
	databasePath, appErr := defaultDatabasePath()
	if appErr != nil {
		return nil, "", "", appErr
	}
	service, appErr := newService(manifestPath, databasePath, nil, requireManifest)
	return service, manifestPath, databasePath, appErr
}

func newService(manifestPath, databasePath string, client HTTPDoer, requireManifest bool) (*Service, *AppError) {
	var manifest *Manifest
	if requireManifest {
		loaded, appErr := loadManifest(manifestPath)
		if appErr != nil {
			return nil, appErr
		}
		manifest = loaded
	}
	store, appErr := openStore(databasePath)
	if appErr != nil {
		return nil, appErr
	}
	return &Service{
		manifest: manifest,
		store:    store,
		adapter:  newHTTPAdapter(client),
	}, nil
}

func (s *Service) Close() error {
	return s.store.Close()
}

func (s *Service) check(manifestPath, databasePath string) CheckResult {
	return CheckResult{
		OK:           true,
		ManifestPath: manifestPath,
		DatabasePath: databasePath,
		ToolCount:    len(s.manifest.Tools),
	}
}

func (s *Service) listTools() []ToolSummary {
	return s.manifest.listTools()
}

func (s *Service) getTool(name string) (ToolDetail, *AppError) {
	return s.manifest.toolDetail(name)
}

func (s *Service) runTool(ctx context.Context, name, rawInput string) (RunResult, *AppError) {
	tool, appErr := s.manifest.getTool(name)
	if appErr != nil {
		return RunResult{}, appErr
	}
	input, inputJSON, appErr := parseJSONObject(rawInput)
	if appErr != nil {
		return RunResult{}, appErr
	}
	fingerprint, appErr := tool.fingerprint()
	if appErr != nil {
		return RunResult{}, appErr
	}
	record := RunRecord{
		ID:              newRunID(),
		ToolName:        name,
		Effect:          tool.Effect,
		ToolFingerprint: fingerprint,
		InputJSON:       inputJSON,
		Status:          StatusCreated,
		RequestedAt:     nowRFC3339(),
	}
	if appErr := s.store.createRun(record); appErr != nil {
		return RunResult{}, appErr
	}

	if appErr := validateInput(tool.InputSchema, input); appErr != nil {
		running, markErr := s.store.markRunning(record.ID, nowRFC3339())
		if markErr != nil {
			return fromRunRecord(record, nil), appErr
		}
		failed := s.failRunningRun(running.ID, appErr)
		return failed, appErr
	}

	if tool.Effect == "write" {
		pending, appErr := s.store.markPending(record.ID)
		if appErr != nil {
			return RunResult{}, appErr
		}
		return fromRunRecord(pending, nil), nil
	}

	running, appErr := s.store.markRunning(record.ID, nowRFC3339())
	if appErr != nil {
		return RunResult{}, appErr
	}
	output, appErr := s.adapter.execute(ctx, tool, input)
	if appErr != nil {
		failed := s.failRunningRun(running.ID, appErr)
		return failed, appErr
	}
	succeeded, appErr := s.store.completeRun(running.ID, StatusSucceeded, 0, nil, nil, nowRFC3339())
	if appErr != nil {
		return RunResult{}, appErr
	}
	return fromRunRecord(succeeded, output), nil
}

func (s *Service) approveRun(ctx context.Context, id string) (RunResult, *AppError) {
	pending, appErr := s.store.getRun(id)
	if appErr != nil {
		return RunResult{}, appErr
	}
	tool, toolErr := s.manifest.getTool(pending.ToolName)
	expectedFingerprint := ""
	if toolErr == nil {
		expectedFingerprint, toolErr = tool.fingerprint()
	}
	if toolErr != nil && toolErr.Code != CodeToolNotFound {
		return RunResult{}, toolErr
	}

	now := nowRFC3339()
	running, appErr := s.store.approveRun(ctx, id, expectedFingerprint, now, now)
	if appErr != nil {
		return fromRunRecord(running, nil), appErr
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(running.InputJSON), &input); err != nil {
		appErr := errorf(CodeValidationError, "stored input is invalid JSON: %v", err)
		failed := s.failRunningRun(running.ID, appErr)
		return failed, appErr
	}
	output, appErr := s.adapter.execute(ctx, tool, input)
	if appErr != nil {
		failed := s.failRunningRun(running.ID, appErr)
		return failed, appErr
	}
	succeeded, appErr := s.store.completeRun(running.ID, StatusSucceeded, 0, nil, nil, nowRFC3339())
	if appErr != nil {
		return RunResult{}, appErr
	}
	return fromRunRecord(succeeded, output), nil
}

func (s *Service) rejectRun(id string) (RunRecord, *AppError) {
	return s.store.rejectRun(id, nowRFC3339())
}

func (s *Service) listRuns(status string) ([]RunRecord, *AppError) {
	return s.store.listRuns(status)
}

func (s *Service) getRun(id string) (RunRecord, *AppError) {
	return s.store.getRun(id)
}

func (s *Service) failRunningRun(id string, reason *AppError) RunResult {
	exitCode := 1
	errorCode := reason.Code
	errorMessage := reason.Message
	record, appErr := s.store.completeRun(id, StatusFailed, exitCode, &errorCode, &errorMessage, nowRFC3339())
	if appErr != nil {
		errorCode = appErr.Code
		errorMessage = appErr.Message
		return RunResult{
			ID:           id,
			Status:       StatusFailed,
			ExitCode:     &exitCode,
			ErrorCode:    &errorCode,
			ErrorMessage: &errorMessage,
		}
	}
	return fromRunRecord(record, nil)
}

func fromRunRecord(record RunRecord, output any) RunResult {
	return RunResult{
		ID:              record.ID,
		ToolName:        record.ToolName,
		Effect:          record.Effect,
		ToolFingerprint: record.ToolFingerprint,
		Status:          record.Status,
		RequestedAt:     record.RequestedAt,
		ApprovedAt:      record.ApprovedAt,
		StartedAt:       record.StartedAt,
		FinishedAt:      record.FinishedAt,
		ExitCode:        record.ExitCode,
		ErrorCode:       record.ErrorCode,
		ErrorMessage:    record.ErrorMessage,
		Output:          output,
	}
}

func newRunID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "run_" + hex.EncodeToString([]byte(nowRFC3339()))[:16]
	}
	return "run_" + hex.EncodeToString(data[:])
}
