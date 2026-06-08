package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func TestHTTPAdapterGETReturnsJSON(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("week") != "current" {
			t.Fatalf("week query = %q", r.URL.Query().Get("week"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{
		Name:          "weekly.get_records",
		Adapter:       "http",
		AdapterConfig: map[string]any{"method": "GET", "url": server.URL + "/records?week=${input.week}"},
	}

	// #when
	out, err := NewHTTPAdapter().Execute(context.Background(), tool, json.RawMessage(`{"week":"current"}`))

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(out) != `{"records":[]}` {
		t.Fatalf("out = %s", out)
	}
}

func TestHTTPAdapterRejectsInputWithTrailingJSONValue(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{
		Name:          "weekly.get_records",
		Adapter:       "http",
		AdapterConfig: map[string]any{"method": "GET", "url": server.URL},
	}

	// #when
	_, err := NewHTTPAdapter().Execute(context.Background(), tool, json.RawMessage(`{} true`))

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input must contain exactly one JSON object" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestNewHTTPAdapterUsesPrivateDefaultTimeout(t *testing.T) {
	// #given
	adapter := NewHTTPAdapter()

	// #when
	client := adapter.client

	// #then
	if client == http.DefaultClient {
		t.Fatal("client = http.DefaultClient")
	}
	if client.Timeout <= 0 {
		t.Fatalf("client.Timeout = %s, want bounded timeout", client.Timeout)
	}
}

func TestHTTPAdapterPOSTSJSONBodyAndHeaders(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if body["week"] != "current" || body["content"] != "done" {
			t.Fatalf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{
		Name:    "weekly.submit_report",
		Adapter: "http",
		Secrets: []string{"WORK_API_TOKEN"},
		AdapterConfig: map[string]any{
			"method":    "POST",
			"url":       server.URL + "/reports",
			"headers":   map[string]any{"Authorization": "Bearer ${secrets.WORK_API_TOKEN}"},
			"json_body": map[string]any{"week": "${input.week}", "content": "${input.content}"},
		},
	}
	env := map[string]string{"WORK_API_TOKEN": "secret-token"}

	// #when
	out, err := NewHTTPAdapter(WithSecrets(env)).Execute(context.Background(), tool, json.RawMessage(`{"week":"current","content":"done"}`))

	// #then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("out = %s", out)
	}
}

func TestHTTPAdapterRejectsUndeclaredSecret(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{
		Name:    "weekly.submit_report",
		Adapter: "http",
		Secrets: []string{"WORK_API_TOKEN"},
		AdapterConfig: map[string]any{
			"method":  "POST",
			"url":     server.URL + "/reports",
			"headers": map[string]any{"Authorization": "Bearer ${secrets.ADMIN_TOKEN}"},
		},
	}
	env := map[string]string{"ADMIN_TOKEN": "do-not-leak", "WORK_API_TOKEN": "secret-token"}

	// #when
	_, err := NewHTTPAdapter(WithSecrets(env)).Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	if err == nil || err.Error() != "MISSING_SECRET: secret ADMIN_TOKEN is required" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHTTPAdapterTimesOutHungServer(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{Name: "weekly.get_records", Adapter: "http", AdapterConfig: map[string]any{"method": "GET", "url": server.URL}}
	client := &http.Client{Timeout: 20 * time.Millisecond}

	// #when
	_, err := NewHTTPAdapter(WithHTTPClient(client)).Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	perr := protocol.AsError(err)
	if perr == nil || perr.Code != protocol.AdapterError {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHTTPAdapterExecutionErrorDoesNotLeakRenderedURLSecret(t *testing.T) {
	// #given
	secretValue := "secret-token-in-url"
	tool := domain.Tool{
		Name:    "weekly.get_records",
		Adapter: "http",
		Secrets: []string{"WORK_API_TOKEN"},
		AdapterConfig: map[string]any{
			"method": "GET",
			"url":    "https://example.invalid/records?token=${secrets.WORK_API_TOKEN}",
		},
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("request failed for %s", req.URL.String())
	})}

	// #when
	_, err := NewHTTPAdapter(WithSecrets(map[string]string{"WORK_API_TOKEN": secretValue}), WithHTTPClient(client)).Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	perr := protocol.AsError(err)
	if perr == nil || perr.Code != protocol.AdapterError {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("Execute() error leaked secret: %v", err)
	}
}

func TestHTTPAdapterRejectsNon2xxStatus(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{Name: "weekly.get_records", Adapter: "http", AdapterConfig: map[string]any{"method": "GET", "url": server.URL}}

	// #when
	_, err := NewHTTPAdapter().Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	if err == nil || err.Error() != "ADAPTER_ERROR: HTTP request failed with status 502" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHTTPAdapterRejectsNonJSONResponse(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{Name: "weekly.get_records", Adapter: "http", AdapterConfig: map[string]any{"method": "GET", "url": server.URL}}

	// #when
	_, err := NewHTTPAdapter().Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	if err == nil || err.Error() != "INVALID_ADAPTER_OUTPUT: HTTP response must be JSON" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHTTPAdapterRejectsResponseWithTrailingData(t *testing.T) {
	// #given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true} false`))
	}))
	t.Cleanup(server.Close)
	tool := domain.Tool{Name: "weekly.get_records", Adapter: "http", AdapterConfig: map[string]any{"method": "GET", "url": server.URL}}

	// #when
	_, err := NewHTTPAdapter().Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	if err == nil || err.Error() != "INVALID_ADAPTER_OUTPUT: HTTP response must be JSON" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestRegistryRejectsUnsupportedAdapter(t *testing.T) {
	// #given
	tool := domain.Tool{Name: "local.echo", Adapter: "shell"}

	// #when
	_, err := NewRegistry().Execute(context.Background(), tool, json.RawMessage(`{}`))

	// #then
	if err == nil || err.Error() != "TOOL_CATALOG_ERROR: unsupported adapter shell" {
		t.Fatalf("Execute() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
