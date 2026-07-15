package anthropic

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

func encodeTextResponse(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          "msg_123",
		"model":       "claude-opus-4-8",
		"stop_reason": "end_turn",
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	})
}

func TestAnalyze_StructuredOutput(t *testing.T) {
	schema := &provider.Schema{
		Name:        "assessment_result",
		Description: "Structured verdict for a repository assessment.",
		Strict:      true,
		Value:       json.RawMessage(`{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"],"additionalProperties":false}`),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("unexpected x-api-key header: %s", got)
		}
		if got := r.Header.Get("anthropic-version"); got != apiVersion {
			t.Fatalf("unexpected anthropic-version header: %s", got)
		}

		var request messagesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "claude-opus-4-8" {
			t.Fatalf("unexpected model: %s", request.Model)
		}
		if request.MaxTokens == 0 {
			t.Fatal("expected max_tokens to be set (required by the Messages API)")
		}
		if request.System != "analyze this repository" {
			t.Fatalf("expected prompt in system field, got %q", request.System)
		}
		if len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != "README content" {
			t.Fatalf("unexpected messages: %#v", request.Messages)
		}
		if request.OutputConfig == nil || request.OutputConfig.Format == nil || request.OutputConfig.Format.Type != "json_schema" {
			t.Fatalf("expected json_schema output format, got %#v", request.OutputConfig)
		}

		w.Header().Set("request-id", "req-123")
		encodeTextResponse(t, w, `{"verdict":"PASS"}`)
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "claude-opus-4-8",
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
	if response.Metadata.FinishReason != "end_turn" {
		t.Fatalf("unexpected finish reason: %s", response.Metadata.FinishReason)
	}
}

func TestAnalyze_OAuthTokenUsesBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-ant-oat01-test" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != oauthBetaHeader {
			t.Fatalf("unexpected anthropic-beta header: %s", got)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Fatalf("expected no x-api-key header for OAuth tokens, got %s", got)
		}
		encodeTextResponse(t, w, "ok")
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "sk-ant-oat01-test",
		Model:    "claude-opus-4-8",
		BaseURL:  server.URL,
	})

	response, err := client.Analyze(context.Background(), "prompt", "content", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Text != "ok" {
		t.Fatalf("unexpected text: %q", response.Text)
	}
}

func TestAnalyze_SkipsNonTextBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_123",
			"model":       "claude-opus-4-8",
			"stop_reason": "end_turn",
			"content": []map[string]string{
				{"type": "thinking", "text": ""},
				{"type": "text", "text": "the answer"},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "claude-opus-4-8",
		BaseURL:  server.URL,
	})

	response, err := client.Analyze(context.Background(), "prompt", "content", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Text != "the answer" {
		t.Fatalf("unexpected text: %q", response.Text)
	}
}

func TestAnalyze_RefusalStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_123",
			"model":       "claude-opus-4-8",
			"stop_reason": "refusal",
			"content":     []map[string]string{},
		})
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "claude-opus-4-8",
		BaseURL:  server.URL,
	})

	_, err := client.Analyze(context.Background(), "prompt", "content", nil)
	if err == nil {
		t.Fatal("expected refusal error, got nil")
	}
	var aiErr *provider.Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *provider.Error, got %T", err)
	}
	if aiErr.Kind != provider.ErrorKindProviderError {
		t.Fatalf("unexpected error kind: %s", aiErr.Kind)
	}
}

func TestAnalyze_RequiresSchemaBody(t *testing.T) {
	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "claude-opus-4-8",
		BaseURL:  "https://example.com",
	})

	_, err := client.Analyze(context.Background(), "prompt", "content", &provider.Schema{Name: "schema-without-body"})
	if err == nil {
		t.Fatal("expected missing schema body error")
	}

	var aiErr *provider.Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *provider.Error, got %T", err)
	}
	if aiErr.Kind != provider.ErrorKindUnsupportedConfig {
		t.Fatalf("unexpected error kind: %s", aiErr.Kind)
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
		{name: "invalid request", statusCode: http.StatusBadRequest, wantKind: provider.ErrorKindInvalidRequest},
		{name: "overloaded", statusCode: 529, wantKind: provider.ErrorKindProviderError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type":  "error",
					"error": map[string]string{"type": "api_error", "message": "provider said no"},
				})
			}))
			defer server.Close()

			client := newTestClient(provider.Config{
				Provider: Provider,
				APIKey:   "test-key",
				Model:    "claude-opus-4-8",
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
			if aiErr.Message != "provider said no" {
				t.Fatalf("expected the error envelope message, got %q", aiErr.Message)
			}
		})
	}
}

func TestAnalyze_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		encodeTextResponse(t, w, "ok")
	}))
	defer server.Close()

	client := newTestClient(provider.Config{
		Provider: Provider,
		APIKey:   "test-key",
		Model:    "claude-opus-4-8",
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
