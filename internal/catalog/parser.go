package catalog

import (
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

func parseToolFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, catalogError("could not read %s", filepath.Base(path))
	}
	return parseToolYAML(filepath.Base(path), data)
}

func parseToolYAML(basename string, data []byte) (map[string]any, error) {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil, unsupportedYAMLError(basename)
	}
	if len(file.Docs) != 1 || file.Docs[0].Body == nil {
		return nil, unsupportedYAMLError(basename)
	}
	if hasUnsupportedYAML(file) {
		return nil, unsupportedYAMLError(basename)
	}
	if _, ok := file.Docs[0].Body.(*ast.MappingNode); !ok {
		return nil, unsupportedYAMLError(basename)
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, unsupportedYAMLError(basename)
	}
	normalized, err := normalizeJSONValue(raw)
	if err != nil {
		return nil, unsupportedYAMLError(basename)
	}
	m, ok := normalized.(map[string]any)
	if !ok {
		return nil, unsupportedYAMLError(basename)
	}
	return m, nil
}

func hasUnsupportedYAML(file *ast.File) bool {
	unsupported := []ast.NodeType{
		ast.AnchorType,
		ast.AliasType,
		ast.DirectiveType,
		ast.MergeKeyType,
		ast.TagType,
	}
	for _, typ := range unsupported {
		if len(ast.FilterFile(typ, file)) > 0 {
			return true
		}
	}
	return false
}

func unsupportedYAMLError(basename string) error {
	return catalogError("unsupported YAML syntax in %s", basename)
}
