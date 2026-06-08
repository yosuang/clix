package domain

import (
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Effect string

const (
	EffectRead  Effect = "read"
	EffectWrite Effect = "write"
)

type Tool struct {
	Version       int            `json:"version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Adapter       string         `json:"adapter"`
	Effect        Effect         `json:"effect"`
	Secrets       []string       `json:"secrets,omitempty"`
	InputSchema   map[string]any `json:"input_schema"`
	OutputSchema  map[string]any `json:"output_schema"`
	AdapterConfig map[string]any `json:"adapter_config,omitempty"`
	SourcePath    string         `json:"source_path,omitempty"`
	Fingerprint   string         `json:"fingerprint,omitempty"`

	inputValidator *jsonschema.Schema
}

func (t Tool) PublicMap() map[string]any {
	return map[string]any{
		"name":          t.Name,
		"description":   t.Description,
		"adapter":       t.Adapter,
		"effect":        t.Effect,
		"input_schema":  t.InputSchema,
		"output_schema": t.OutputSchema,
	}
}

func (t Tool) WithInputValidator(validator *jsonschema.Schema) Tool {
	t.inputValidator = validator
	return t
}

func (t Tool) HasInputValidator() bool {
	return t.inputValidator != nil
}

func (t Tool) ValidateInput(input any) error {
	if t.inputValidator == nil {
		return errors.New("input validator is not configured")
	}
	return t.inputValidator.Validate(input)
}
