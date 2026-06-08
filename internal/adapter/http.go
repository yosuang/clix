package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

type HTTPOption func(*HTTPAdapter)

type HTTPAdapter struct {
	client  *http.Client
	secrets map[string]string
}

const defaultHTTPTimeout = 30 * time.Second

func NewHTTPAdapter(options ...HTTPOption) *HTTPAdapter {
	adapter := &HTTPAdapter{client: &http.Client{Timeout: defaultHTTPTimeout}, secrets: map[string]string{}}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithSecrets(secrets map[string]string) HTTPOption {
	return func(adapter *HTTPAdapter) {
		adapter.secrets = secrets
	}
}

func WithHTTPClient(client *http.Client) HTTPOption {
	return func(adapter *HTTPAdapter) {
		if client != nil {
			adapter.client = client
		}
	}
}

func (a *HTTPAdapter) Execute(ctx context.Context, tool domain.Tool, input json.RawMessage) (json.RawMessage, error) {
	secrets := a.secretsForTool(tool)

	method, ok := tool.AdapterConfig["method"].(string)
	if !ok || method == "" {
		return nil, protocol.NewError(protocol.ToolCatalogError, tool.Name+" http.method is required")
	}

	urlTemplate, ok := tool.AdapterConfig["url"].(string)
	if !ok || urlTemplate == "" {
		return nil, protocol.NewError(protocol.ToolCatalogError, tool.Name+" http.url is required")
	}
	url, err := renderTemplate(urlTemplate, input, secrets)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	hasBody := false
	if rawBody, exists := tool.AdapterConfig["json_body"]; exists {
		renderedBody, err := renderJSONValue(rawBody, input, secrets)
		if err != nil {
			return nil, err
		}
		bodyBytes, err := json.Marshal(renderedBody)
		if err != nil {
			return nil, protocol.NewError(protocol.AdapterError, err.Error())
		}
		body = bytes.NewReader(bodyBytes)
		hasBody = true
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, protocol.NewError(protocol.AdapterError, err.Error())
	}

	headers, err := renderHeaders(tool.AdapterConfig["headers"], input, secrets)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, protocol.NewError(protocol.AdapterError, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, protocol.NewError(protocol.AdapterError, fmt.Sprintf("HTTP request failed with status %d", resp.StatusCode))
	}

	out, err := decodeJSONResponse(resp.Body)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *HTTPAdapter) secretsForTool(tool domain.Tool) map[string]string {
	secrets := make(map[string]string, len(tool.Secrets))
	for _, name := range tool.Secrets {
		if value, ok := a.secrets[name]; ok {
			secrets[name] = value
		}
	}
	return secrets
}

func renderHeaders(raw any, input json.RawMessage, secrets map[string]string) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}

	switch headers := raw.(type) {
	case map[string]any:
		rendered := make(map[string]string, len(headers))
		for key, value := range headers {
			template, ok := value.(string)
			if !ok {
				return nil, protocol.NewError(protocol.ToolCatalogError, "http.headers."+key+" must be a string")
			}
			header, err := renderTemplate(template, input, secrets)
			if err != nil {
				return nil, err
			}
			rendered[key] = header
		}
		return rendered, nil
	case map[string]string:
		rendered := make(map[string]string, len(headers))
		for key, template := range headers {
			header, err := renderTemplate(template, input, secrets)
			if err != nil {
				return nil, err
			}
			rendered[key] = header
		}
		return rendered, nil
	default:
		return nil, protocol.NewError(protocol.ToolCatalogError, "http.headers must be a map")
	}
}

func renderJSONValue(value any, input json.RawMessage, secrets map[string]string) (any, error) {
	switch value := value.(type) {
	case string:
		return renderTemplate(value, input, secrets)
	case []any:
		rendered := make([]any, 0, len(value))
		for _, item := range value {
			renderedItem, err := renderJSONValue(item, input, secrets)
			if err != nil {
				return nil, err
			}
			rendered = append(rendered, renderedItem)
		}
		return rendered, nil
	case map[string]any:
		rendered := make(map[string]any, len(value))
		for key, item := range value {
			renderedItem, err := renderJSONValue(item, input, secrets)
			if err != nil {
				return nil, err
			}
			rendered[key] = renderedItem
		}
		return rendered, nil
	default:
		return value, nil
	}
}

func decodeJSONResponse(body io.Reader) (json.RawMessage, error) {
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, protocol.NewError(protocol.InvalidAdapterOutput, "HTTP response must be JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, protocol.NewError(protocol.InvalidAdapterOutput, "HTTP response must be JSON")
	}
	rendered, err := json.Marshal(decoded)
	if err != nil {
		return nil, protocol.NewError(protocol.InvalidAdapterOutput, "HTTP response must be JSON")
	}
	return rendered, nil
}
