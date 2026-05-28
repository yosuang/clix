package clix

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type Manifest struct {
	Version int                       `json:"version" yaml:"version"`
	Tools   map[string]ToolDefinition `json:"tools" yaml:"tools"`
}

type ToolDefinition struct {
	Description  string         `json:"description" yaml:"description"`
	Adapter      string         `json:"adapter" yaml:"adapter"`
	Effect       string         `json:"effect" yaml:"effect"`
	Secrets      []string       `json:"secrets,omitempty" yaml:"secrets"`
	InputSchema  map[string]any `json:"input_schema" yaml:"input_schema"`
	OutputSchema map[string]any `json:"output_schema" yaml:"output_schema"`
	HTTP         *HTTPConfig    `json:"http,omitempty" yaml:"http"`
}

type HTTPConfig struct {
	Method   string            `json:"method" yaml:"method"`
	URL      string            `json:"url" yaml:"url"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers"`
	JSONBody any               `json:"json_body,omitempty" yaml:"json_body"`
}

type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Adapter     string `json:"adapter"`
	Effect      string `json:"effect"`
}

type ToolDetail struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Adapter      string         `json:"adapter"`
	Effect       string         `json:"effect"`
	Secrets      []string       `json:"secrets,omitempty"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`
	HTTP         *HTTPConfig    `json:"http,omitempty"`
}

func loadManifest(path string) (*Manifest, *AppError) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errorf(CodeManifestError, "create manifest directory: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errorf(CodeManifestError, "load manifest: %v", err)
	}
	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errorf(CodeManifestError, "parse manifest: %v", err)
	}
	if appErr := manifest.normalize(); appErr != nil {
		return nil, appErr
	}
	if appErr := manifest.validate(); appErr != nil {
		return nil, appErr
	}
	return &manifest, nil
}

func (m *Manifest) normalize() *AppError {
	if m.Tools == nil {
		return nil
	}
	for name, tool := range m.Tools {
		inputSchema, appErr := canonicalMap(tool.InputSchema)
		if appErr != nil {
			return errorf(CodeManifestError, "tool %q input_schema: %s", name, appErr.Message)
		}
		outputSchema, appErr := canonicalMap(tool.OutputSchema)
		if appErr != nil {
			return errorf(CodeManifestError, "tool %q output_schema: %s", name, appErr.Message)
		}
		tool.InputSchema = inputSchema
		tool.OutputSchema = outputSchema
		if tool.HTTP != nil && tool.HTTP.JSONBody != nil {
			body, appErr := canonicalAny(tool.HTTP.JSONBody)
			if appErr != nil {
				return errorf(CodeManifestError, "tool %q http.json_body: %s", name, appErr.Message)
			}
			tool.HTTP.JSONBody = body
		}
		m.Tools[name] = tool
	}
	return nil
}

func (m *Manifest) validate() *AppError {
	if m.Version != 1 {
		return errorf(CodeManifestError, "unsupported manifest version %d", m.Version)
	}
	if m.Tools == nil {
		return newError(CodeManifestError, "tools is required")
	}
	for name, tool := range m.Tools {
		if !toolNamePattern.MatchString(name) {
			return errorf(CodeManifestError, "invalid tool name %q", name)
		}
		if tool.Description == "" {
			return errorf(CodeManifestError, "tool %q description is required", name)
		}
		if tool.Adapter != "http" {
			return errorf(CodeManifestError, "tool %q uses unsupported adapter %q", name, tool.Adapter)
		}
		if tool.Effect != "read" && tool.Effect != "write" {
			return errorf(CodeManifestError, "tool %q effect must be read or write", name)
		}
		if tool.InputSchema == nil {
			return errorf(CodeManifestError, "tool %q input_schema is required", name)
		}
		if tool.OutputSchema == nil {
			return errorf(CodeManifestError, "tool %q output_schema is required", name)
		}
		if appErr := validateSchemaSubset(tool.InputSchema, "input_schema"); appErr != nil {
			return errorf(CodeManifestError, "tool %q %s", name, appErr.Message)
		}
		if appErr := validateSchemaSubset(tool.OutputSchema, "output_schema"); appErr != nil {
			return errorf(CodeManifestError, "tool %q %s", name, appErr.Message)
		}
		if schemaType, _ := tool.InputSchema["type"].(string); schemaType != "object" {
			return errorf(CodeManifestError, "tool %q input_schema.type must be object", name)
		}
		if tool.HTTP == nil {
			return errorf(CodeManifestError, "tool %q http config is required", name)
		}
		if tool.HTTP.Method == "" {
			return errorf(CodeManifestError, "tool %q http.method is required", name)
		}
		if tool.HTTP.URL == "" {
			return errorf(CodeManifestError, "tool %q http.url is required", name)
		}
	}
	return nil
}

func (m *Manifest) getTool(name string) (ToolDefinition, *AppError) {
	tool, ok := m.Tools[name]
	if !ok {
		return ToolDefinition{}, errorf(CodeToolNotFound, "tool %q not found", name)
	}
	return tool, nil
}

func (m *Manifest) listTools() []ToolSummary {
	names := sortedKeys(m.Tools)
	items := make([]ToolSummary, 0, len(names))
	for _, name := range names {
		tool := m.Tools[name]
		items = append(items, ToolSummary{
			Name:        name,
			Description: tool.Description,
			Adapter:     tool.Adapter,
			Effect:      tool.Effect,
		})
	}
	return items
}

func (m *Manifest) toolDetail(name string) (ToolDetail, *AppError) {
	tool, appErr := m.getTool(name)
	if appErr != nil {
		return ToolDetail{}, appErr
	}
	return ToolDetail{
		Name:         name,
		Description:  tool.Description,
		Adapter:      tool.Adapter,
		Effect:       tool.Effect,
		Secrets:      tool.Secrets,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
		HTTP:         tool.HTTP,
	}, nil
}

func (t ToolDefinition) fingerprint() (string, *AppError) {
	data, err := json.Marshal(t)
	if err != nil {
		return "", errorf(CodeManifestError, "fingerprint tool: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalMap(value map[string]any) (map[string]any, *AppError) {
	if value == nil {
		return nil, nil
	}
	normalized, appErr := canonicalAny(value)
	if appErr != nil {
		return nil, appErr
	}
	result, ok := normalized.(map[string]any)
	if !ok {
		return nil, newError(CodeManifestError, "must be an object")
	}
	return result, nil
}

func canonicalAny(value any) (any, *AppError) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errorf(CodeManifestError, "%v", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, errorf(CodeManifestError, "%v", err)
	}
	return normalized, nil
}
