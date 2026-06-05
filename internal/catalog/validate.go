package catalog

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yosuang/clix/internal/domain"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func validateTool(sourcePath string, raw map[string]any, validator AdapterValidator) (domain.Tool, error) {
	version, ok := intField(raw, "version")
	if !ok || version != 1 {
		return domain.Tool{}, catalogError("version must be 1")
	}

	name, ok := stringField(raw, "name")
	if !ok || !toolNamePattern.MatchString(name) {
		return domain.Tool{}, catalogError("invalid tool name")
	}

	description, ok := stringField(raw, "description")
	if !ok || strings.TrimSpace(description) == "" {
		return domain.Tool{}, catalogError("description is required")
	}

	adapter, ok := stringField(raw, "adapter")
	if !ok || adapter != "http" {
		return domain.Tool{}, catalogError("adapter must be http")
	}

	effectText, ok := stringField(raw, "effect")
	if !ok {
		return domain.Tool{}, catalogError("effect is required")
	}
	effect := domain.Effect(effectText)
	if effect != domain.EffectRead && effect != domain.EffectWrite {
		return domain.Tool{}, catalogError("effect must be read or write")
	}

	inputSchema, ok := mapField(raw, "input_schema")
	if !ok {
		return domain.Tool{}, catalogError("input_schema is required")
	}
	outputSchema, ok := mapField(raw, "output_schema")
	if !ok {
		return domain.Tool{}, catalogError("output_schema is required")
	}
	adapterConfig, ok := mapField(raw, adapter)
	if !ok {
		return domain.Tool{}, catalogError("%s adapter config is required", adapter)
	}
	secrets, ok := secretsField(raw)
	if !ok {
		return domain.Tool{}, catalogError("declared secrets must be strings")
	}

	if err := validateSchemaSubset(name, inputSchema); err != nil {
		return domain.Tool{}, err
	}
	if err := validateInputSchemaRoot(name, inputSchema); err != nil {
		return domain.Tool{}, err
	}
	if err := validateSchemaSubset(name, outputSchema); err != nil {
		return domain.Tool{}, err
	}
	inputValidator, err := compileInputSchema(name, inputSchema)
	if err != nil {
		return domain.Tool{}, err
	}
	if err := validateHTTPAdapterConfig(name, adapterConfig); err != nil {
		return domain.Tool{}, err
	}
	if err := validator.ValidateAdapter(adapter, adapterConfig); err != nil {
		return domain.Tool{}, catalogError("invalid %s adapter config in %s: %s", adapter, name, err)
	}

	tool := domain.Tool{
		Version:       version,
		Name:          name,
		Description:   description,
		Adapter:       adapter,
		Effect:        effect,
		Secrets:       secrets,
		InputSchema:   inputSchema,
		OutputSchema:  outputSchema,
		AdapterConfig: adapterConfig,
		SourcePath:    sourcePath,
	}
	fingerprint, err := fingerprintTool(tool)
	if err != nil {
		return domain.Tool{}, err
	}
	tool.Fingerprint = fingerprint
	return tool.WithInputValidator(inputValidator), nil
}

func validateHTTPAdapterConfig(toolName string, config map[string]any) error {
	supported := map[string]struct{}{
		"method":    {},
		"url":       {},
		"headers":   {},
		"json_body": {},
	}
	for key := range config {
		if _, ok := supported[key]; !ok {
			return catalogError("%s http.%s is not supported", toolName, key)
		}
	}

	method, ok := config["method"].(string)
	if !ok || strings.TrimSpace(method) == "" {
		return catalogError("%s http.method is required", toolName)
	}
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return catalogError("%s http.method must be one of GET, POST, PUT, PATCH, DELETE", toolName)
	}

	url, ok := config["url"].(string)
	if !ok || strings.TrimSpace(url) == "" {
		return catalogError("%s http.url is required", toolName)
	}

	if headers, exists := config["headers"]; exists {
		headerMap, ok := headers.(map[string]any)
		if !ok {
			return catalogError("%s http.headers must be a map", toolName)
		}
		for key, value := range headerMap {
			if _, ok := value.(string); !ok {
				return catalogError("%s http.headers.%s must be a string", toolName, key)
			}
		}
	}

	if body, exists := config["json_body"]; exists && !isJSONConfigValue(body) {
		return catalogError("%s http.json_body must be JSON", toolName)
	}
	return nil
}

func isJSONConfigValue(value any) bool {
	switch value := value.(type) {
	case nil, bool, string, float64:
		return true
	case json.Number:
		return true
	case []any:
		for _, item := range value {
			if !isJSONConfigValue(item) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, item := range value {
			if !isJSONConfigValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func stringField(raw map[string]any, key string) (string, bool) {
	value, ok := raw[key].(string)
	return value, ok
}

func intField(raw map[string]any, key string) (int, bool) {
	switch value := raw[key].(type) {
	case json.Number:
		i, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), i == int64(int(i))
	case float64:
		i := int(value)
		return i, value == float64(i)
	case int:
		return value, true
	default:
		return 0, false
	}
}

func mapField(raw map[string]any, key string) (map[string]any, bool) {
	value, ok := raw[key].(map[string]any)
	return value, ok
}

func secretsField(raw map[string]any) ([]string, bool) {
	value, exists := raw["secrets"]
	if !exists {
		return nil, true
	}
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	secrets := make([]string, 0, len(items))
	for _, item := range items {
		secret, ok := item.(string)
		if !ok {
			return nil, false
		}
		secrets = append(secrets, secret)
	}
	return secrets, true
}
