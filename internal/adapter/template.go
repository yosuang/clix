package adapter

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/yosuang/clix/internal/protocol"
)

var templateExpressionPattern = regexp.MustCompile(`\$\{([^}]*)\}`)

func renderTemplate(template string, input json.RawMessage, secrets map[string]string) (string, error) {
	matches := templateExpressionPattern.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		if strings.Contains(template, "${") {
			return "", unsupportedTemplateExpression()
		}
		return template, nil
	}

	inputFields, err := inputObject(input)
	if err != nil {
		return "", err
	}

	var rendered strings.Builder
	last := 0
	for _, match := range matches {
		if strings.Contains(template[last:match[0]], "${") {
			return "", unsupportedTemplateExpression()
		}
		rendered.WriteString(template[last:match[0]])
		expression := template[match[2]:match[3]]
		value, err := renderExpression(expression, inputFields, secrets)
		if err != nil {
			return "", err
		}
		rendered.WriteString(value)
		last = match[1]
	}
	if strings.Contains(template[last:], "${") {
		return "", unsupportedTemplateExpression()
	}
	rendered.WriteString(template[last:])
	return rendered.String(), nil
}

func renderExpression(expression string, input map[string]any, secrets map[string]string) (string, error) {
	if strings.HasPrefix(expression, "input.") {
		field := strings.TrimPrefix(expression, "input.")
		if !isTemplateName(field) {
			return "", protocol.NewError(protocol.ValidationError, "unsupported template expression")
		}
		value, ok := input[field]
		if !ok {
			return "", protocol.NewError(protocol.ValidationError, "input."+field+" is required")
		}
		return templateValueString(value)
	}

	if strings.HasPrefix(expression, "secrets.") {
		name := strings.TrimPrefix(expression, "secrets.")
		if !isTemplateName(name) {
			return "", protocol.NewError(protocol.ValidationError, "unsupported template expression")
		}
		value, ok := secrets[name]
		if !ok {
			return "", protocol.NewError(protocol.MissingSecret, "secret "+name+" is required")
		}
		return value, nil
	}

	return "", protocol.NewError(protocol.ValidationError, "unsupported template expression")
}

func unsupportedTemplateExpression() error {
	return protocol.NewError(protocol.ValidationError, "unsupported template expression")
}

func inputObject(input json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return nil, protocol.NewError(protocol.ValidationError, "input must be valid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, protocol.NewError(protocol.ValidationError, "input must be valid JSON")
	}
	if fields == nil {
		fields = map[string]any{}
	}
	return fields, nil
}

func templateValueString(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "null", nil
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case json.Number:
		return value.String(), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	default:
		rendered, err := json.Marshal(value)
		if err != nil {
			return "", protocol.NewError(protocol.ValidationError, "input is invalid")
		}
		return string(rendered), nil
	}
}

func isTemplateName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return true
}
