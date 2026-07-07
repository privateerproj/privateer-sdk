package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// testProvider stands in for a concrete adapter identity; provider tests must
// not import adapter packages (they import this one).
const testProvider Provider = "openai"

func TestBaseNewJSONRequest_AppliesOptions(t *testing.T) {
	base := NewBase(testProvider, Config{}, "https://example.com/v1")
	req, err := base.NewJSONRequest(
		context.Background(),
		http.MethodPost,
		"/responses",
		map[string]string{"prompt": "hello"},
		RequestOptions{
			Headers: map[string]string{"Authorization": "Bearer test-key"},
			Query:   url.Values{"key": []string{"abc123"}},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.String() != "https://example.com/v1/responses?key=abc123" {
		t.Fatalf("unexpected request URL: %s", req.URL.String())
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("unexpected authorization header: %s", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type: %s", got)
	}
	var body map[string]string
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body["prompt"] != "hello" {
		t.Fatalf("unexpected body content: %#v", body)
	}
}

// NewBase must normalize the config itself: adapter constructors are exported
// and callable with a raw config, so the registry's normalization cannot be
// assumed. A zero Timeout reaching http.Client would mean no timeout at all.
func TestNewBase_NormalizesConfig(t *testing.T) {
	base := NewBase(testProvider, Config{Provider: " OpenAI ", APIKey: " key "}, "https://example.com/v1")
	if base.Config.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want default %v", base.Config.Timeout, defaultTimeout)
	}
	if base.Config.MaxTokens != defaultMaxTokens {
		t.Errorf("MaxTokens = %d, want default %d", base.Config.MaxTokens, defaultMaxTokens)
	}
	if base.Config.Provider != "openai" || base.Config.APIKey != "key" {
		t.Errorf("Provider/APIKey = %q/%q, want trimmed and lowercased", base.Config.Provider, base.Config.APIKey)
	}
	if base.httpClient == nil || base.httpClient.Timeout != defaultTimeout {
		t.Errorf("httpClient timeout = %v, want %v", base.httpClient.Timeout, defaultTimeout)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := normalizeBaseURL(" https://api.example.com/v1/ ", "https://default.example.com"); got != "https://api.example.com/v1" {
		t.Fatalf("unexpected normalized url: %s", got)
	}
	if got := normalizeBaseURL("", "https://default.example.com"); got != "https://default.example.com" {
		t.Fatalf("unexpected default url: %s", got)
	}
}

func TestValidateStructuredSchema(t *testing.T) {
	if err := ValidateStructuredSchema(testProvider, nil); err != nil {
		t.Fatalf("unexpected nil schema error: %v", err)
	}
	if err := ValidateStructuredSchema(testProvider, &Schema{Name: "schema-without-body"}); err == nil {
		t.Fatal("expected missing schema body error")
	}
	if err := ValidateStructuredSchema(testProvider, &Schema{Value: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatalf("unexpected error for nameless schema: %v", err)
	}
	if err := ValidateStructuredSchema(testProvider, &Schema{Name: "assessment_result", Value: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatalf("unexpected valid schema error: %v", err)
	}
}

func TestParseStructuredOutput(t *testing.T) {
	raw, err := ParseStructuredOutput(testProvider, "  {\"verdict\":\"PASS\"}  ")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if string(raw) != `{"verdict":"PASS"}` {
		t.Fatalf("unexpected parsed content: %s", raw)
	}

	_, err = ParseStructuredOutput(testProvider, "not-json")
	if err == nil {
		t.Fatal("expected invalid json error")
	}
	var aiErr *Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if aiErr.Kind != ErrorKindInvalidResponse {
		t.Fatalf("unexpected error kind: %s", aiErr.Kind)
	}
	if !strings.Contains(aiErr.Error(), "openai structured response") {
		t.Fatalf("unexpected error message: %s", aiErr.Error())
	}
}
