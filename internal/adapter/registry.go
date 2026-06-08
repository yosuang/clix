package adapter

import (
	"context"
	"encoding/json"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

type Adapter interface {
	Execute(context.Context, domain.Tool, json.RawMessage) (json.RawMessage, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(options ...HTTPOption) *Registry {
	return &Registry{adapters: map[string]Adapter{"http": NewHTTPAdapter(options...)}}
}

func (r *Registry) Execute(ctx context.Context, tool domain.Tool, input json.RawMessage) (json.RawMessage, error) {
	adapter, ok := r.adapters[tool.Adapter]
	if !ok {
		return nil, protocol.NewError(protocol.ToolCatalogError, "unsupported adapter "+tool.Adapter)
	}
	return adapter.Execute(ctx, tool, input)
}
