package cmdutil

import (
	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/iostreams"
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

type Factory struct {
	IO            *iostreams.IOStreams
	Output        OutputOptions
	CatalogLoader CatalogLoader
}
