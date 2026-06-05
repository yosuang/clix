package catalog

import "github.com/yosuang/clix/internal/domain"

type AdapterValidator interface {
	ValidateAdapter(adapter string, config map[string]any) error
}

type Options struct {
	ToolsDir         string
	AdapterValidator AdapterValidator
}

type Catalog struct {
	Tools  []domain.Tool
	ByName map[string]domain.Tool
}

func (c Catalog) Get(name string) (domain.Tool, bool) {
	tool, ok := c.ByName[name]
	return tool, ok
}

type Loader struct {
	Options Options
}

func NewLoader(options Options) Loader {
	return Loader{Options: options}
}

func (l Loader) Load() (Catalog, error) {
	return Load(l.Options)
}

func emptyCatalog() Catalog {
	return Catalog{Tools: []domain.Tool{}, ByName: map[string]domain.Tool{}}
}

type acceptAdapterValidator struct{}

func (acceptAdapterValidator) ValidateAdapter(string, map[string]any) error {
	return nil
}
