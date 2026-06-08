package runservice

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

type Store interface {
	InsertRun(context.Context, domain.Run) error
	GetRun(context.Context, string) (domain.Run, error)
	ClaimPendingRun(context.Context, string, time.Time) (domain.Run, error)
	MarkSucceeded(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, time.Time, protocol.Code, string) error
	MarkRejected(context.Context, string, time.Time) error
}

type Catalog interface {
	Get(string) (domain.Tool, bool)
	ValidateInput(string, json.RawMessage) error
}

type AdapterRegistry interface {
	Execute(context.Context, domain.Tool, json.RawMessage) (json.RawMessage, error)
}

type IDSource interface {
	NewRunID() (string, error)
}

type ServiceOptions struct {
	Store    Store
	Catalog  Catalog
	Adapters AdapterRegistry
	IDs      IDSource
	Now      func() time.Time
}

type Service struct {
	store    Store
	catalog  Catalog
	adapters AdapterRegistry
	ids      IDSource
	now      func() time.Time
}

type Result struct {
	Run    domain.Run
	Output json.RawMessage
}

func New(opts ServiceOptions) *Service {
	ids := opts.IDs
	if ids == nil {
		defaultIDs := NewIDGenerator()
		ids = defaultIDs
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		store:    opts.Store,
		catalog:  opts.Catalog,
		adapters: opts.Adapters,
		ids:      ids,
		now:      now,
	}
}

func (s *Service) Run(ctx context.Context, toolName string, input json.RawMessage) (Result, error) {
	tool, ok := s.catalog.Get(toolName)
	if !ok {
		return Result{}, protocol.NewError(protocol.ToolNotFound, "tool not found")
	}
	if err := s.catalog.ValidateInput(toolName, input); err != nil {
		return Result{}, err
	}

	id, err := s.ids.NewRunID()
	if err != nil {
		return Result{}, protocol.NewError(protocol.InternalError, err.Error())
	}
	requestedAt := s.now()
	run := domain.Run{
		ID:              id,
		ToolName:        tool.Name,
		Effect:          tool.Effect,
		ToolFingerprint: tool.Fingerprint,
		ToolSourcePath:  tool.SourcePath,
		InputJSON:       append([]byte(nil), input...),
		Status:          domain.StatusPendingApproval,
		RequestedAt:     requestedAt,
	}
	if tool.Effect == domain.EffectRead {
		startedAt := requestedAt
		run.Status = domain.StatusRunning
		run.StartedAt = &startedAt
	}
	if err := s.store.InsertRun(ctx, run); err != nil {
		return Result{}, err
	}
	if tool.Effect == domain.EffectWrite {
		return Result{Run: run}, nil
	}

	return s.execute(ctx, run, tool)
}

func (s *Service) Approve(ctx context.Context, runID string) (Result, error) {
	startedAt := s.now()
	run, err := s.store.ClaimPendingRun(ctx, runID, startedAt)
	if err != nil {
		return Result{}, err
	}
	tool, ok := s.catalog.Get(run.ToolName)
	if !ok || tool.Fingerprint != run.ToolFingerprint {
		err := protocol.NewError(protocol.ToolChanged, "tool definition changed before approval")
		if markErr := s.store.MarkFailed(ctx, run.ID, s.now(), err.Code, err.Message); markErr != nil {
			return Result{}, markErr
		}
		return Result{}, err
	}
	return s.execute(ctx, run, tool)
}

func (s *Service) Reject(ctx context.Context, runID string) (domain.Run, error) {
	if err := s.store.MarkRejected(ctx, runID, s.now()); err != nil {
		return domain.Run{}, err
	}
	return s.store.GetRun(ctx, runID)
}

func (s *Service) execute(ctx context.Context, run domain.Run, tool domain.Tool) (Result, error) {
	output, err := s.adapters.Execute(ctx, tool, json.RawMessage(run.InputJSON))
	finishedAt := s.now()
	if err != nil {
		perr := protocol.AsError(err)
		if markErr := s.store.MarkFailed(ctx, run.ID, finishedAt, perr.Code, perr.Message); markErr != nil {
			return Result{}, markErr
		}
		failed, getErr := s.store.GetRun(ctx, run.ID)
		if getErr != nil {
			return Result{}, getErr
		}
		return Result{Run: failed}, err
	}
	if err := s.store.MarkSucceeded(ctx, run.ID, finishedAt); err != nil {
		return Result{}, err
	}
	succeeded, err := s.store.GetRun(ctx, run.ID)
	if err != nil {
		return Result{}, err
	}
	return Result{Run: succeeded, Output: output}, nil
}
