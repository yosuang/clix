package cmdutil

import (
	"context"
	"time"

	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

type OutputOptions struct {
	JSONFields []string
	JSONSet    bool
	JQ         string
	JQSet      bool
}

type CatalogLoader interface {
	Load() (catalog.Catalog, error)
}

type RunStore interface {
	InsertRun(context.Context, domain.Run) error
	GetRun(context.Context, string) (domain.Run, error)
	ListRuns(context.Context, *domain.Status) ([]domain.Run, error)
	ClaimPendingRun(context.Context, string, time.Time) (domain.Run, error)
	MarkSucceeded(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, time.Time, protocol.Code, string) error
	MarkRejected(context.Context, string, time.Time) error
}

type Factory struct {
	IO            *iostreams.IOStreams
	Output        OutputOptions
	CatalogLoader CatalogLoader
	RunStore      RunStore
}
