package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

func TestToolsListPrintsTextRows(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io, CatalogLoader: &staticCatalogLoader{catalog: testCatalog()}}
	root := NewRoot(f)
	root.SetArgs([]string{"tools", "list"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "weekly.get_records read http - Get work records for a given week.\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestToolsGetPrintsTextSummary(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io, CatalogLoader: &staticCatalogLoader{catalog: testCatalog()}}
	root := NewRoot(f)
	root.SetArgs([]string{"tools", "get", "weekly.get_records"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "weekly.get_records read http\nGet work records for a given week.\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestToolsListProjectsJSONPublicFields(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io, CatalogLoader: &staticCatalogLoader{catalog: testCatalog()}}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "name,effect,adapter", "tools", "list"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "[{\"adapter\":\"http\",\"effect\":\"read\",\"name\":\"weekly.get_records\"}]\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestToolsListRejectsUnknownJSONFieldForEmptyCatalog(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io, CatalogLoader: &staticCatalogLoader{catalog: catalog.Catalog{Tools: []domain.Tool{}, ByName: map[string]domain.Tool{}}}}
	root := NewRoot(f)
	root.SetArgs([]string{"--json", "missing", "tools", "list"})

	// #when
	err := root.Execute()

	// #then
	if err == nil || err.Error() != `USAGE_ERROR: unknown field "missing"` {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestToolsGetUnknownToolReturnsToolNotFound(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	f := &cmdutil.Factory{IO: io, CatalogLoader: &staticCatalogLoader{catalog: testCatalog()}}
	root := NewRoot(f)
	root.SetArgs([]string{"tools", "get", "weekly.missing"})

	// #when
	err := root.Execute()

	// #then
	if err == nil {
		t.Fatal("Execute() error = nil, want tool not found")
	}
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("Execute() error = %T, want protocol error", err)
	}
	if perr.Code != protocol.ToolNotFound {
		t.Fatalf("error code = %q, want %q", perr.Code, protocol.ToolNotFound)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, command layer must not print errors", stderr.String())
	}
}

func TestCheckLoadsCatalogBeforePrintingOK(t *testing.T) {
	// #given
	var stdout, stderr bytes.Buffer
	io := iostreams.TestIO(nil, &stdout, &stderr, true)
	loader := &staticCatalogLoader{catalog: testCatalog()}
	f := &cmdutil.Factory{IO: io, CatalogLoader: loader}
	root := NewRoot(f)
	root.SetArgs([]string{"check"})

	// #when
	err := root.Execute()

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !loader.called {
		t.Fatal("catalog loader was not called")
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok newline", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func testCatalog() catalog.Catalog {
	tool := domain.Tool{
		Version:     1,
		Name:        "weekly.get_records",
		Description: "Get work records for a given week.",
		Adapter:     "http",
		Effect:      domain.EffectRead,
		InputSchema: map[string]any{
			"type": "object",
		},
		OutputSchema: map[string]any{
			"type": "object",
		},
	}
	return catalog.Catalog{
		Tools:  []domain.Tool{tool},
		ByName: map[string]domain.Tool{tool.Name: tool},
	}
}

type staticCatalogLoader struct {
	catalog catalog.Catalog
	err     error
	called  bool
}

func (l *staticCatalogLoader) Load() (catalog.Catalog, error) {
	l.called = true
	return l.catalog, l.err
}
