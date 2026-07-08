package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/privateerproj/privateer-sdk/ai/provider"
)

// newTestClient mirrors the registry path: adapters receive a normalized config.
func newTestClient(config provider.Config) *Client {
	return NewClient(config.Normalized())
}

func TestNewClient_AppliesDefaultMaxTokens(t *testing.T) {
	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	if client.Config.MaxTokens == 0 {
		t.Fatal("expected normalized config to carry a default max tokens, got 0")
	}
}

func TestAnalyze_StructuredOutput(t *testing.T) {
	schema := &provider.Schema{
		Name:        "assessment_result",
		Description: "Structured verdict for a repository assessment.",
		Strict:      true,
		Value:       json.RawMessage(`{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"],"additionalProperties":false}`),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		var request chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "gpt-4o-mini" {
			t.Fatalf("unexpected model: %s", request.Model)
		}
		if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected json schema response format, got %#v", request.ResponseFormat)
		}
		if request.ResponseFormat.JSONSchema == nil || request.ResponseFormat.JSONSchema.Name != schema.Name {
			t.Fatalf("unexpected schema wrapper: %#v", request.ResponseFormat.JSONSchema)
		}

		w.Header().Set("x-request-id", "req-123")
		_ = json.NewEncoder(w).Encode(chatCompletionsResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4o-mini",
			Choices: []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					FinishReason: "stop",
					Message: struct {
						Content string `json:"content"`
					}{Content: `{"verdict":"PASS"}`},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
		BaseURL:  server.URL,
	})

	response, err := client.Analyze(context.Background(), "analyze this repository", "README content", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(response.JSON) != `{"verdict":"PASS"}` {
		t.Fatalf("unexpected JSON payload: %s", response.JSON)
	}
	if response.Metadata.Provider != Provider {
		t.Fatalf("unexpected provider: %s", response.Metadata.Provider)
	}
	if response.Metadata.RequestID != "req-123" {
		t.Fatalf("unexpected request id: %s", response.Metadata.RequestID)
	}
}

func TestAnalyze_RequiresSchemaName(t *testing.T) {
	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
		BaseURL:  "https://example.com",
	})

	_, err := client.Analyze(context.Background(), "prompt", "content", &provider.Schema{
		Value: json.RawMessage(`{"type":"object"}`),
	})
	if err == nil {
		t.Fatal("expected missing schema name error")
	}

	var aiErr *provider.Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *provider.Error, got %T", err)
	}
	if aiErr.Kind != provider.ErrorKindUnsupportedConfig {
		t.Fatalf("unexpected error kind: %s", aiErr.Kind)
	}
	if aiErr.Message != "structured output schema name is required" {
		t.Fatalf("unexpected error message: %s", aiErr.Message)
	}
}

func TestAnalyze_HTTPErrorKinds(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantKind   provider.ErrorKind
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantKind: provider.ErrorKindUnauthorized},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantKind: provider.ErrorKindRateLimited},
		{name: "provider error", statusCode: http.StatusServiceUnavailable, wantKind: provider.ErrorKindProviderError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": "provider said no"},
				})
			}))
			defer server.Close()

			client := newTestClient(provider.Config{
				Provider: Provider,
				APIKey:   "test-key",
				Model:    "gpt-4o-mini",
				BaseURL:  server.URL,
			})

			_, err := client.Analyze(context.Background(), "prompt", "content", nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var aiErr *provider.Error
			if !errors.As(err, &aiErr) {
				t.Fatalf("expected *provider.Error, got %T", err)
			}
			if aiErr.Kind != tt.wantKind {
				t.Fatalf("expected error kind %s, got %s", tt.wantKind, aiErr.Kind)
			}
		})
	}
}

func TestAnalyze_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-123","model":"gpt-4o-mini","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
		BaseURL:  server.URL,
		Timeout:  10 * time.Millisecond,
	})

	_, err := client.Analyze(context.Background(), "prompt", "content", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	var aiErr *provider.Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *provider.Error, got %T", err)
	}
	if aiErr.Kind != provider.ErrorKindTimeout {
		t.Fatalf("expected timeout error kind, got %s", aiErr.Kind)
	}
}
