package catalog

import (
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var supportedSchemaKeywords = map[string]struct{}{
	"type":                 {},
	"properties":           {},
	"required":             {},
	"items":                {},
	"enum":                 {},
	"minimum":              {},
	"maximum":              {},
	"minLength":            {},
	"maxLength":            {},
	"additionalProperties": {},
}

func validateSchemaSubset(toolName string, schema map[string]any) error {
	return validateSchemaObject(toolName, schema)
}

func validateInputSchemaRoot(toolName string, schema map[string]any) error {
	schemaType, ok := schema["type"].(string)
	if !ok || schemaType != "object" {
		return catalogError("input_schema.type must be object in %s", toolName)
	}
	return nil
}

func validateSchemaObject(toolName string, schema map[string]any) error {
	for keyword, value := range schema {
		if _, ok := supportedSchemaKeywords[keyword]; !ok {
			return catalogError("unsupported schema keyword %q in %s", keyword, toolName)
		}
		switch keyword {
		case "type":
			if _, ok := value.(string); !ok {
				return catalogError("invalid schema in %s", toolName)
			}
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return catalogError("invalid schema in %s", toolName)
			}
			for _, propertySchema := range properties {
				if err := validateNestedSchema(toolName, propertySchema); err != nil {
					return err
				}
			}
		case "items":
			if err := validateNestedSchema(toolName, value); err != nil {
				return err
			}
		case "additionalProperties":
			switch nested := value.(type) {
			case bool:
			case map[string]any:
				if err := validateSchemaObject(toolName, nested); err != nil {
					return err
				}
			default:
				return catalogError("invalid schema in %s", toolName)
			}
		case "required":
			items, ok := value.([]any)
			if !ok {
				return catalogError("invalid schema in %s", toolName)
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return catalogError("invalid schema in %s", toolName)
				}
			}
		case "enum":
			if _, ok := value.([]any); !ok {
				return catalogError("invalid schema in %s", toolName)
			}
		case "minimum", "maximum", "minLength", "maxLength":
			if !isSchemaNumber(value) {
				return catalogError("invalid schema in %s", toolName)
			}
		}
	}
	return nil
}

func validateNestedSchema(toolName string, value any) error {
	nested, ok := value.(map[string]any)
	if !ok {
		return catalogError("invalid schema in %s", toolName)
	}
	return validateSchemaObject(toolName, nested)
}

func isSchemaNumber(value any) bool {
	switch value.(type) {
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func compileInputSchema(toolName string, schema map[string]any) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("input_schema.json", schema); err != nil {
		return nil, catalogError("invalid input_schema in %s", toolName)
	}
	compiled, err := compiler.Compile("input_schema.json")
	if err != nil {
		return nil, catalogError("invalid input_schema in %s", toolName)
	}
	return compiled, nil
}
