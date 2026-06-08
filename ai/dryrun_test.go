package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_DryRunReturnsDryRunClient(t *testing.T) {
	client, err := NewClient(Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o-mini",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*dryRunClient); !ok {
		t.Fatalf("expected *dryRunClient, got %T", client)
	}
}

func TestNewClient_DryRunDoesNotRequireAPIKey(t *testing.T) {
	if _, err := NewClient(Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o-mini",
		DryRun:   true,
	}); err != nil {
		t.Fatalf("expected dry-run to succeed without api key, got: %v", err)
	}
}

func TestNewClient_DryRunStillRequiresProviderAndModel(t *testing.T) {
	if _, err := NewClient(Config{DryRun: true, Model: "gpt-4o-mini"}); err == nil {
		t.Fatal("expected error when provider missing in dry-run, got nil")
	}
	if _, err := NewClient(Config{DryRun: true, Provider: ProviderOpenAI}); err == nil {
		t.Fatal("expected error when model missing in dry-run, got nil")
	}
}

func TestNewClient_DryRunRejectsUnsupportedProvider(t *testing.T) {
	// Typos should surface in dry-run mode too, so users don't discover them
	// only after switching to a live run.
	_, err := NewClient(Config{
		Provider: "totally-bogus",
		Model:    "gpt-4o-mini",
		DryRun:   true,
	})
	if err == nil {
		t.Fatal("expected unsupported-provider error in dry-run, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported ai provider") {
		t.Fatalf("expected unsupported-provider error, got: %v", err)
	}
}

func TestDryRunAnalyze_ReturnsSentinelMetadata(t *testing.T) {
	client := newDryRunClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini", MaxTokens: 256})
	var buf bytes.Buffer
	client.logOutput = &buf

	resp, err := client.Analyze(context.Background(), "what is 2+2?", "context-doc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Metadata.FinishReason != FinishReasonDryRun {
		t.Fatalf("expected FinishReason %q, got %q", FinishReasonDryRun, resp.Metadata.FinishReason)
	}
	if resp.Metadata.Provider != ProviderOpenAI {
		t.Fatalf("expected provider %q, got %q", ProviderOpenAI, resp.Metadata.Provider)
	}
	if resp.Metadata.Model != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %q", resp.Metadata.Model)
	}
	if !strings.Contains(resp.Text, "[ai dry-run]") {
		t.Fatalf("expected dry-run marker in Text, got %q", resp.Text)
	}
}

func TestDryRunAnalyze_LogsPromptDetails(t *testing.T) {
	client := newDryRunClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini", MaxTokens: 128})
	var buf bytes.Buffer
	client.logOutput = &buf

	schema := &Schema{Name: "verdict", Value: json.RawMessage(`{"type":"object"}`)}
	if _, err := client.Analyze(context.Background(), "PROMPT-X", "CONTENT-Y", schema); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logged := buf.String()
	for _, want := range []string{"PROMPT-X", "CONTENT-Y", "gpt-4o-mini", "openai", "max_tokens=128", "schema=verdict"} {
		if !strings.Contains(logged, want) {
			t.Errorf("expected log output to contain %q; got:\n%s", want, logged)
		}
	}
}

func TestDryRunAnalyze_MakesNoHTTPCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o-mini",
		DryRun:   true,
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := client.Analyze(context.Background(), "p", "c", nil); err != nil {
		t.Fatalf("unexpected analyze error: %v", err)
	}
	if called {
		t.Fatal("dry-run should not make any HTTP request")
	}
}

func TestDryRunAnalyze_RespectsCancelledContext(t *testing.T) {
	client := newDryRunClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Analyze(ctx, "p", "c", nil); err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestDryRunAnalyze_LogsNoSchemaAsPlaceholder(t *testing.T) {
	client := newDryRunClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini"})
	var buf bytes.Buffer
	client.logOutput = &buf

	if _, err := client.Analyze(context.Background(), "p", "c", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "schema=<none>") {
		t.Errorf("expected 'schema=<none>' in log output, got:\n%s", buf.String())
	}
}

func TestNewClient_DryRunFalseReturnsRealAdapter(t *testing.T) {
	client, err := NewClient(Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o-mini",
		APIKey:   "test-key",
		// DryRun left at zero value (false).
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*dryRunClient); ok {
		t.Fatal("expected real adapter when DryRun=false, got *dryRunClient")
	}
	if _, ok := client.(*openAIClient); !ok {
		t.Fatalf("expected *openAIClient when DryRun=false, got %T", client)
	}
}

func TestDryRunAnalyze_RejectsInvalidSchema(t *testing.T) {
	// Malformed schemas should fail in dry-run with the same error a live
	// adapter would return, so callers don't discover the typo only after
	// switching off dry-run. This covers both the package-level check
	// (missing Value) and the OpenAI-specific rule (missing Name), which
	// dry-run reaches by delegating to the provider's ValidateRequest.
	client := newDryRunClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini"})
	cases := map[string]*Schema{
		"missing Value": {Name: "foo"},
		"missing Name":  {Value: json.RawMessage(`{"type":"object"}`)},
	}
	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Analyze(context.Background(), "p", "c", schema)
			if err == nil {
				t.Fatal("expected schema-validation error in dry-run, got nil")
			}
			var aiErr *Error
			if !errors.As(err, &aiErr) || aiErr.Kind != ErrorKindUnsupportedConfig {
				t.Fatalf("expected ErrorKindUnsupportedConfig, got %v", err)
			}
		})
	}
}
