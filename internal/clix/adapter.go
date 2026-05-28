package clix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var placeholderPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPAdapter struct {
	client HTTPDoer
}

func newHTTPAdapter(client HTTPDoer) *HTTPAdapter {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPAdapter{client: client}
}

func (a *HTTPAdapter) execute(ctx context.Context, tool ToolDefinition, input map[string]any) (any, *AppError) {
	secrets := declaredSecrets(tool.Secrets)
	method := strings.ToUpper(tool.HTTP.Method)
	url, appErr := substituteString(tool.HTTP.URL, input, secrets)
	if appErr != nil {
		return nil, appErr
	}

	var body io.Reader
	headers := map[string]string{}
	for name, value := range tool.HTTP.Headers {
		resolved, appErr := substituteString(value, input, secrets)
		if appErr != nil {
			return nil, appErr
		}
		headers[name] = resolved
	}
	if tool.HTTP.JSONBody != nil {
		resolvedBody, appErr := substituteJSON(tool.HTTP.JSONBody, input, secrets)
		if appErr != nil {
			return nil, appErr
		}
		data, err := json.Marshal(resolvedBody)
		if err != nil {
			return nil, errorf(CodeAdapterError, "encode JSON body: %v", err)
		}
		body = bytes.NewReader(data)
		if !hasHeader(headers, "content-type") {
			headers["Content-Type"] = "application/json"
		}
	}

	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, errorf(CodeAdapterError, "create HTTP request: %v", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := a.client.Do(request)
	if err != nil {
		return nil, errorf(CodeAdapterError, "HTTP request failed: %v", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 10*1024*1024))
	if err != nil {
		return nil, errorf(CodeAdapterError, "read HTTP response: %v", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errorf(CodeAdapterError, "HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	var output any
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, errorf(CodeInvalidAdapterOutput, "HTTP response must be JSON: %v", err)
	}
	return output, nil
}

func declaredSecrets(names []string) map[string]bool {
	secrets := make(map[string]bool, len(names))
	for _, name := range names {
		secrets[name] = true
	}
	return secrets
}

func substituteJSON(value any, input map[string]any, secrets map[string]bool) (any, *AppError) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, appErr := substituteJSON(item, input, secrets)
			if appErr != nil {
				return nil, appErr
			}
			result[key] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			resolved, appErr := substituteJSON(item, input, secrets)
			if appErr != nil {
				return nil, appErr
			}
			result = append(result, resolved)
		}
		return result, nil
	case string:
		if expr, ok := exactPlaceholder(typed); ok {
			return resolvePlaceholder(expr, input, secrets)
		}
		return substituteString(typed, input, secrets)
	default:
		return typed, nil
	}
}

func substituteString(value string, input map[string]any, secrets map[string]bool) (string, *AppError) {
	var appErr *AppError
	result := placeholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		if appErr != nil {
			return ""
		}
		expr := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		resolved, err := resolvePlaceholder(expr, input, secrets)
		if err != nil {
			appErr = err
			return ""
		}
		text, err := scalarString(resolved)
		if err != nil {
			appErr = err
			return ""
		}
		return text
	})
	if appErr != nil {
		return "", appErr
	}
	return result, nil
}

func exactPlaceholder(value string) (string, bool) {
	matches := placeholderPattern.FindStringSubmatch(value)
	if len(matches) != 2 || matches[0] != value {
		return "", false
	}
	return matches[1], true
}

func resolvePlaceholder(expr string, input map[string]any, secrets map[string]bool) (any, *AppError) {
	switch {
	case strings.HasPrefix(expr, "input."):
		path := strings.Split(strings.TrimPrefix(expr, "input."), ".")
		if len(path) == 0 || path[0] == "" {
			return nil, newError(CodeValidationError, "input placeholder path is empty")
		}
		var current any = input
		for _, segment := range path {
			object, ok := current.(map[string]any)
			if !ok {
				return nil, errorf(CodeValidationError, "input.%s is not an object", strings.Join(path, "."))
			}
			next, ok := object[segment]
			if !ok {
				return nil, errorf(CodeValidationError, "input.%s is missing", strings.Join(path, "."))
			}
			current = next
		}
		return current, nil
	case strings.HasPrefix(expr, "secrets."):
		name := strings.TrimPrefix(expr, "secrets.")
		if name == "" {
			return nil, newError(CodeMissingSecret, "secret name is empty")
		}
		if !secrets[name] {
			return nil, errorf(CodeManifestError, "secret %q is used but not declared", name)
		}
		value, ok := lookupEnv(name)
		if !ok {
			return nil, errorf(CodeMissingSecret, "environment variable %s is not set", name)
		}
		return value, nil
	default:
		return nil, errorf(CodeManifestError, "unsupported placeholder %q", expr)
	}
}

func scalarString(value any) (string, *AppError) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case float64, bool:
		return fmt.Sprint(typed), nil
	case nil:
		return "", nil
	default:
		return "", errorf(CodeValidationError, "cannot substitute %T into a string", value)
	}
}

func hasHeader(headers map[string]string, target string) bool {
	for name := range headers {
		if strings.EqualFold(name, target) {
			return true
		}
	}
	return false
}

var lookupEnv = func(name string) (string, bool) {
	return os.LookupEnv(name)
}
