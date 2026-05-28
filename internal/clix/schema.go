package clix

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
)

var supportedSchemaKeywords = map[string]bool{
	"additionalProperties": true,
	"enum":                 true,
	"items":                true,
	"maxLength":            true,
	"maximum":              true,
	"minLength":            true,
	"minimum":              true,
	"properties":           true,
	"required":             true,
	"type":                 true,
}

func validateSchemaSubset(schema map[string]any, path string) *AppError {
	for keyword, value := range schema {
		if !supportedSchemaKeywords[keyword] {
			return errorf(CodeManifestError, "%s uses unsupported schema keyword %q", path, keyword)
		}
		switch keyword {
		case "type":
			typeName, ok := value.(string)
			if !ok {
				return errorf(CodeManifestError, "%s.type must be a string", path)
			}
			if !supportedSchemaTypes[typeName] {
				return errorf(CodeManifestError, "%s.type %q is not supported", path, typeName)
			}
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return errorf(CodeManifestError, "%s.properties must be an object", path)
			}
			for name, child := range properties {
				childSchema, ok := child.(map[string]any)
				if !ok {
					return errorf(CodeManifestError, "%s.properties.%s must be an object", path, name)
				}
				if appErr := validateSchemaSubset(childSchema, path+".properties."+name); appErr != nil {
					return appErr
				}
			}
		case "required":
			if _, ok := stringSlice(value); !ok {
				return errorf(CodeManifestError, "%s.required must be an array of strings", path)
			}
		case "items":
			childSchema, ok := value.(map[string]any)
			if !ok {
				return errorf(CodeManifestError, "%s.items must be an object", path)
			}
			if appErr := validateSchemaSubset(childSchema, path+".items"); appErr != nil {
				return appErr
			}
		case "enum":
			if _, ok := value.([]any); !ok {
				return errorf(CodeManifestError, "%s.enum must be an array", path)
			}
		case "minimum", "maximum":
			if _, ok := numberValue(value); !ok {
				return errorf(CodeManifestError, "%s.%s must be a number", path, keyword)
			}
		case "minLength", "maxLength":
			if _, ok := integerValue(value); !ok {
				return errorf(CodeManifestError, "%s.%s must be an integer", path, keyword)
			}
		case "additionalProperties":
			if _, ok := value.(bool); !ok {
				return errorf(CodeManifestError, "%s.additionalProperties must be a boolean", path)
			}
		}
	}
	return nil
}

var supportedSchemaTypes = map[string]bool{
	"array":   true,
	"boolean": true,
	"integer": true,
	"null":    true,
	"number":  true,
	"object":  true,
	"string":  true,
}

func parseJSONObject(raw string) (map[string]any, string, *AppError) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, "", errorf(CodeValidationError, "input must be valid JSON: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", newError(CodeValidationError, "input must contain one JSON value")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, "", newError(CodeValidationError, "input must be a JSON object")
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, "", errorf(CodeValidationError, "canonicalize input: %v", err)
	}
	return object, string(data), nil
}

func validateInput(schema map[string]any, value map[string]any) *AppError {
	return validateValue(schema, value, "input")
}

func validateValue(schema map[string]any, value any, path string) *AppError {
	if enumValues, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range enumValues {
			if reflect.DeepEqual(candidate, value) {
				matched = true
				break
			}
		}
		if !matched {
			return errorf(CodeValidationError, "%s must be one of the allowed enum values", path)
		}
	}
	if typeName, ok := schema["type"].(string); ok {
		if appErr := validateType(typeName, value, path); appErr != nil {
			return appErr
		}
	}
	switch typeName, _ := schema["type"].(string); typeName {
	case "object":
		object, _ := value.(map[string]any)
		return validateObject(schema, object, path)
	case "array":
		items, _ := value.([]any)
		return validateArray(schema, items, path)
	case "string":
		text, _ := value.(string)
		return validateString(schema, text, path)
	case "number", "integer":
		number, _ := numberValue(value)
		return validateNumber(schema, number, path)
	default:
		return nil
	}
}

func validateType(typeName string, value any, path string) *AppError {
	ok := false
	switch typeName {
	case "object":
		_, ok = value.(map[string]any)
	case "array":
		_, ok = value.([]any)
	case "string":
		_, ok = value.(string)
	case "number":
		_, ok = numberValue(value)
	case "integer":
		number, numberOK := numberValue(value)
		ok = numberOK && math.Trunc(number) == number
	case "boolean":
		_, ok = value.(bool)
	case "null":
		ok = value == nil
	default:
		return errorf(CodeManifestError, "%s has unsupported schema type %q", path, typeName)
	}
	if !ok {
		return errorf(CodeValidationError, "%s must be %s", path, typeName)
	}
	return nil
}

func validateObject(schema map[string]any, value map[string]any, path string) *AppError {
	if required, ok := stringSlice(schema["required"]); ok {
		for _, name := range required {
			if _, ok := value[name]; !ok {
				return errorf(CodeValidationError, "%s.%s is required", path, name)
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, child := range properties {
		if actual, ok := value[name]; ok {
			childSchema, _ := child.(map[string]any)
			if appErr := validateValue(childSchema, actual, path+"."+name); appErr != nil {
				return appErr
			}
		}
	}
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		known := map[string]bool{}
		for name := range properties {
			known[name] = true
		}
		for name := range value {
			if !known[name] {
				return errorf(CodeValidationError, "%s.%s is not allowed", path, name)
			}
		}
	}
	return nil
}

func validateArray(schema map[string]any, items []any, path string) *AppError {
	itemSchema, ok := schema["items"].(map[string]any)
	if !ok {
		return nil
	}
	for index, item := range items {
		if appErr := validateValue(itemSchema, item, fmt.Sprintf("%s[%d]", path, index)); appErr != nil {
			return appErr
		}
	}
	return nil
}

func validateString(schema map[string]any, text string, path string) *AppError {
	if minimum, ok := integerValue(schema["minLength"]); ok && len(text) < int(minimum) {
		return errorf(CodeValidationError, "%s must be at least %d characters", path, int(minimum))
	}
	if maximum, ok := integerValue(schema["maxLength"]); ok && len(text) > int(maximum) {
		return errorf(CodeValidationError, "%s must be at most %d characters", path, int(maximum))
	}
	return nil
}

func validateNumber(schema map[string]any, number float64, path string) *AppError {
	if minimum, ok := numberValue(schema["minimum"]); ok && number < minimum {
		return errorf(CodeValidationError, "%s must be at least %v", path, minimum)
	}
	if maximum, ok := numberValue(schema["maximum"]); ok && number > maximum {
		return errorf(CodeValidationError, "%s must be at most %v", path, maximum)
	}
	return nil
}

func stringSlice(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func integerValue(value any) (float64, bool) {
	number, ok := numberValue(value)
	return number, ok && math.Trunc(number) == number
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
