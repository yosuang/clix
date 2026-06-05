package catalog

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func Load(options Options) (Catalog, error) {
	info, err := os.Stat(options.ToolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyCatalog(), nil
		}
		return emptyCatalog(), catalogError("could not read tools directory")
	}
	if !info.IsDir() {
		return emptyCatalog(), catalogError("tools path is not a directory")
	}

	validator := options.AdapterValidator
	if validator == nil {
		validator = acceptAdapterValidator{}
	}

	paths, err := toolFilePaths(options.ToolsDir)
	if err != nil {
		return emptyCatalog(), err
	}

	tools := make([]domain.Tool, 0, len(paths))
	byName := make(map[string]domain.Tool, len(paths))
	for _, path := range paths {
		raw, err := parseToolFile(path)
		if err != nil {
			return emptyCatalog(), err
		}
		tool, err := validateTool(path, raw, validator)
		if err != nil {
			return emptyCatalog(), err
		}
		if _, exists := byName[tool.Name]; exists {
			return emptyCatalog(), catalogError("duplicate tool name %q", tool.Name)
		}
		tools = append(tools, tool)
		byName[tool.Name] = tool
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return Catalog{Tools: tools, ByName: byName}, nil
}

func toolFilePaths(root string) ([]string, error) {
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return catalogError("could not read tools directory")
		}
		if path != root && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func catalogError(format string, args ...any) error {
	return protocol.NewError(protocol.ToolCatalogError, fmt.Sprintf(format, args...))
}
