package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProviderClientNewJSONRequest_AppliesOptions(t *testing.T) {
	client := newProviderClient(ProviderOpenAI, Config{}, "https://example.com/v1")
	req, err := client.newJSONRequest(
		context.Background(),
		http.MethodPost,
		"/responses",
		map[string]string{"prompt": "hello"},
		requestOptions{
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

func TestNormalizeBaseURL(t *testing.T) {
	if got := normalizeBaseURL(" https://api.example.com/v1/ ", "https://default.example.com"); got != "https://api.example.com/v1" {
		t.Fatalf("unexpected normalized url: %s", got)
	}
	if got := normalizeBaseURL("", "https://default.example.com"); got != "https://default.example.com" {
		t.Fatalf("unexpected default url: %s", got)
	}
}

func TestValidateStructuredSchema(t *testing.T) {
	if err := validateStructuredSchema(ProviderOpenAI, nil); err != nil {
		t.Fatalf("unexpected nil schema error: %v", err)
	}
	if err := validateStructuredSchema(ProviderOpenAI, &Schema{Name: "schema-without-body"}); err == nil {
		t.Fatal("expected missing schema body error")
	}
	if err := validateStructuredSchema(ProviderOpenAI, &Schema{Value: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatalf("unexpected error for nameless schema: %v", err)
	}
	if err := validateStructuredSchema(ProviderOpenAI, &Schema{Name: "assessment_result", Value: json.RawMessage(`{"type":"object"}`)}); err != nil {
		t.Fatalf("unexpected valid schema error: %v", err)
	}
}

func TestParseStructuredOutput(t *testing.T) {
	raw, err := parseStructuredOutput(ProviderOpenAI, "  {\"verdict\":\"PASS\"}  ")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if string(raw) != `{"verdict":"PASS"}` {
		t.Fatalf("unexpected parsed content: %s", raw)
	}

	_, err = parseStructuredOutput(ProviderOpenAI, "not-json")
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
