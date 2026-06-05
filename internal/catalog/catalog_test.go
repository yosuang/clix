package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCatalogLoadsValidToolFilesInDeterministicOrder(t *testing.T) {
	// #given
	dir := t.TempDir()
	writeTool(t, dir, filepath.Join("b", "second.yml"), validTool("zeta.read", "https://example.com/zeta"))
	writeTool(t, dir, filepath.Join("a", "first.yaml"), validTool("alpha.read", "https://example.com/alpha"))
	validator := &acceptHTTP{}

	// #when
	catalog, err := Load(Options{ToolsDir: dir, AdapterValidator: validator})

	// #then
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(catalog.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(catalog.Tools))
	}
	if catalog.Tools[0].Name != "alpha.read" || catalog.Tools[1].Name != "zeta.read" {
		t.Fatalf("Tools order = [%s %s], want sorted by tool name", catalog.Tools[0].Name, catalog.Tools[1].Name)
	}
	if _, ok := catalog.Get("alpha.read"); !ok {
		t.Fatal("Get(alpha.read) ok = false, want true")
	}
	if len(validator.urls) != 2 || validator.urls[0] != "https://example.com/alpha" || validator.urls[1] != "https://example.com/zeta" {
		t.Fatalf("adapter validation URLs = %#v, want lexical path order", validator.urls)
	}
	if catalog.Tools[0].Fingerprint == "" || catalog.Tools[1].Fingerprint == "" {
		t.Fatal("fingerprint is empty, want SHA-256 hex")
	}
}

func TestLoadCatalogMissingDirectoryIsEmptyCatalog(t *testing.T) {
	// #given
	dir := filepath.Join(t.TempDir(), "missing")

	// #when
	catalog, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(catalog.Tools) != 0 {
		t.Fatalf("len(Tools) = %d, want 0", len(catalog.Tools))
	}
	if len(catalog.ByName) != 0 {
		t.Fatalf("len(ByName) = %d, want 0", len(catalog.ByName))
	}
}

func TestLoadCatalogRejectsDuplicateNames(t *testing.T) {
	// #given
	dir := t.TempDir()
	writeTool(t, dir, "one.yaml", validTool("weekly.get_records", "https://example.com/one"))
	writeTool(t, dir, "two.yaml", validTool("weekly.get_records", "https://example.com/two"))

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want duplicate name error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: duplicate tool name "weekly.get_records"` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogRejectsUnsupportedYAMLAnchors(t *testing.T) {
	// #given
	dir := t.TempDir()
	writeTool(t, dir, "tool.yaml", strings.ReplaceAll(validTool("weekly.get_records", "https://example.com/records"), "http:\n", "http: &http\n"))

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported YAML error")
	}
	if got := err.Error(); got != "TOOL_CATALOG_ERROR: unsupported YAML syntax in tool.yaml" {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogRejectsUnsupportedSchemaKeyword(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.ReplaceAll(validTool("weekly.get_records", "https://example.com/records"), "type: string\n", "type: string\n      format: date\n")
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported schema keyword error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: unsupported schema keyword "format" in weekly.get_records` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogRejectsNonObjectInputSchema(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "input_schema:\n  type: object", "input_schema:\n  type: string", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want input_schema type error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: input_schema.type must be object in weekly.get_records` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogStoresCompiledInputValidator(t *testing.T) {
	// #given
	dir := t.TempDir()
	writeTool(t, dir, "tool.yaml", validTool("weekly.get_records", "https://example.com/records"))

	// #when
	catalog, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tool := catalog.Tools[0]
	if !tool.HasInputValidator() {
		t.Fatal("HasInputValidator() = false, want true")
	}
	if err := tool.ValidateInput(map[string]any{"week": "current"}); err != nil {
		t.Fatalf("ValidateInput(valid) error = %v", err)
	}
	if err := tool.ValidateInput(map[string]any{}); err == nil {
		t.Fatal("ValidateInput(missing required) error = nil, want validation error")
	}
}

func TestLoadCatalogRejectsMalformedHTTPConfig(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "  method: GET\n", "", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want http config error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: weekly.get_records http.method is required` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogPreservesLargeIntegerPrecision(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "  url: https://example.com/records\n", "  url: https://example.com/records\n  json_body:\n    id: 9007199254740993\n", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	catalog, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	jsonBody := catalog.Tools[0].AdapterConfig["json_body"].(map[string]any)
	id, ok := jsonBody["id"].(json.Number)
	if !ok {
		t.Fatalf("id = %T, want json.Number", jsonBody["id"])
	}
	if id.String() != "9007199254740993" {
		t.Fatalf("id = %q, want preserved precision", id.String())
	}
}

func TestLoadCatalogRejectsBooleanPropertySchema(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "    week:\n      type: string\n", "    week: true\n", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want invalid schema error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: invalid schema in weekly.get_records` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogRejectsArrayItemsSchema(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "    week:\n      type: string\n", "    week:\n      type: array\n      items:\n        - type: string\n", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want invalid schema error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: invalid schema in weekly.get_records` {
		t.Fatalf("Load() error = %q", got)
	}
}

func TestLoadCatalogRejectsUnknownHTTPConfigField(t *testing.T) {
	// #given
	dir := t.TempDir()
	tool := strings.Replace(validTool("weekly.get_records", "https://example.com/records"), "  url: https://example.com/records\n", "  url: https://example.com/records\n  timeout: 3\n", 1)
	writeTool(t, dir, "tool.yaml", tool)

	// #when
	_, err := Load(Options{ToolsDir: dir, AdapterValidator: &acceptHTTP{}})

	// #then
	if err == nil {
		t.Fatal("Load() error = nil, want unknown http field error")
	}
	if got := err.Error(); got != `TOOL_CATALOG_ERROR: weekly.get_records http.timeout is not supported` {
		t.Fatalf("Load() error = %q", got)
	}
}

func validTool(name string, url string) string {
	return fmt.Sprintf(`version: 1
name: %s
description: Get work records for a given week.
adapter: http
effect: read
secrets:
  - WORK_API_TOKEN
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
  url: %s
`, name, url)
}

func writeTool(t *testing.T, dir string, name string, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

type acceptHTTP struct {
	urls []string
}

func (v *acceptHTTP) ValidateAdapter(adapter string, config map[string]any) error {
	if adapter != "http" {
		return fmt.Errorf("adapter = %q, want http", adapter)
	}
	url, _ := config["url"].(string)
	v.urls = append(v.urls, url)
	return nil
}
