// Package ai defines the provider-neutral contract callers use to talk to
// any AI backend. Concrete providers (e.g. OpenAI) are implemented as
// adapters that satisfy the Client interface and are registered in
// clientFactories. Callers should use only the shared types in this package,
// so adding or changing the SDK's built-in providers does not require changes
// in the code that calls the SDK.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Provider identifies the AI backend implementation. Each supported provider
// gets its own constant and a matching entry in clientFactories.
type Provider string

const (
	ProviderOpenAI Provider = "openai"
	// Additional providers (e.g. Anthropic, Gemini) should be added here
	// alongside a corresponding entry in clientFactories.
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 256

	// FinishReasonDryRun is the predictable FinishReason returned by
	// the dry-run client so callers can distinguish a dry-run result
	// from any real provider response.
	FinishReasonDryRun = "dry_run"
)

// clientFactory builds a provider adapter from a normalized Config. Each
// adapter lives in its own file (e.g. openai.go) and is wired in below.
type clientFactory func(Config) Client

// clientFactories is the internal registry for the providers that ship with
// this SDK. To add another built-in provider, implement Client and register
// its constructor here. Code that uses the SDK does not need to change.
var clientFactories = map[Provider]clientFactory{
	ProviderOpenAI: func(config Config) Client {
		return newOpenAIClient(config)
	},
}

// Client is the provider-neutral analysis contract. Every adapter must
// satisfy it so callers can swap providers without touching call sites.
type Client interface {
	Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error)
}

// Config holds the provider-neutral settings used to construct any adapter.
// Provider-specific concerns (auth header shape, endpoint paths, request body
// schema) are the adapter's responsibility, not this struct's.
type Config struct {
	// Provider selects which adapter is constructed via clientFactories.
	Provider Provider
	// APIKey is the credential passed to the provider. Adapters decide how
	// it is attached to outbound requests (e.g. Bearer header, x-api-key).
	APIKey string
	// Model is the provider-specific model identifier (e.g. "gpt-4o-mini").
	// Lets callers pick the cost/quality tradeoff appropriate for the task.
	Model string
	// BaseURL overrides the adapter's default endpoint. Primarily useful for
	// tests, proxies, and self-hosted gateways. Empty means use the default.
	BaseURL string
	// Timeout bounds a single Analyze call so a slow or hung provider cannot
	// stall the caller indefinitely. Zero falls back to defaultTimeout.
	Timeout time.Duration
	// MaxTokens caps the response length, bounding cost and latency per call.
	// Zero falls back to defaultMaxTokens.
	MaxTokens int
	// HTTPClient lets callers inject a custom transport (e.g. for tests or
	// instrumentation). When nil, a client honoring Timeout is constructed.
	HTTPClient *http.Client
	// DryRun routes Analyze through a logging-only client that never
	// contacts the provider. Useful for inspecting prompts and model
	// settings without spending tokens or needing real credentials.
	DryRun bool
}

// Schema tells the provider to return a structured JSON answer instead of
// free-form text. Callers describe the exact shape they want with a JSON
// Schema document, and the adapter translates that into whatever the
// underlying provider expects (OpenAI's response_format json_schema,
// Anthropic tool input schemas, Gemini's responseSchema, etc.). When a
// Schema is supplied, AnalyzeResponse.JSON holds the parsed payload, ready
// to json.Unmarshal into a Go type.
//
// Example: ask the model to grade a repository's documentation and return
// a verdict the caller can act on programmatically.
//
//	schema := &ai.Schema{
//	    Name:        "doc_grade",
//	    Description: "Grade for a repository's documentation quality.",
//	    Strict:      true,
//	    Value: json.RawMessage(`{
//	        "type": "object",
//	        "properties": {
//	            "verdict":    {"type": "string", "enum": ["pass", "fail"]},
//	            "confidence": {"type": "number"},
//	            "reason":     {"type": "string"}
//	        },
//	        "required": ["verdict", "confidence", "reason"],
//	        "additionalProperties": false
//	    }`),
//	}
//
//	resp, err := client.Analyze(ctx, prompt, readme, schema)
//	// resp.JSON: {"verdict":"pass","confidence":0.82,"reason":"..."}
type Schema struct {
	// Name labels the schema for the provider (some providers display or
	// log it). Think of it as the form's title. Required when Schema is
	// supplied.
	Name string
	// Description is a short sentence telling the model what the schema is
	// for. Optional, but improves output quality when field names alone
	// are ambiguous. Passed through to providers that support it.
	Description string
	// Value is the JSON Schema document the response must conform to —
	// the "form definition." Callers typically json.Unmarshal
	// AnalyzeResponse.JSON into a Go type derived from this schema.
	Value json.RawMessage
	// Strict asks the provider to reject responses that do not match Value
	// exactly ("strict form-filling") instead of doing its best.
	// Providers that do not support strict mode ignore this flag.
	Strict bool
}

// AnalyzeResponse is the normalized result returned by every adapter.
// When Analyze is called without a Schema, only Text is populated. When a
// Schema is supplied, JSON holds the parsed structured payload and Text
// holds the raw message content the provider returned.
type AnalyzeResponse struct {
	// Text is the raw text content of the model's response. Use this for
	// free-form prompts where no structured output was requested.
	Text string
	// JSON is the parsed structured payload. Non-nil only when the caller
	// supplied a Schema and the provider produced valid JSON. Callers can
	// json.Unmarshal it into a typed Go value matching the schema.
	JSON json.RawMessage
	// Metadata describes the response in provider-neutral terms.
	Metadata ResponseMetadata
}

// ResponseMetadata is diagnostic metadata about an Analyze call. The SDK
// does not read these fields itself — they are returned so callers can log
// them, persist them in audit trails, attribute results when juggling
// multiple clients, or branch on signals like FinishReason. Adapters
// translate each provider's native response details into this uniform
// shape so callers do not need provider-specific knowledge.
type ResponseMetadata struct {
	// Provider is the adapter that produced the response. Useful when a
	// caller mixes clients from multiple providers and wants to attribute
	// each result.
	Provider Provider
	// Model is the model name the provider reports having used. It may
	// differ from Config.Model when the requested name is an alias that
	// the provider resolves to a specific pinned version (e.g.
	// "gpt-4o-mini" -> "gpt-4o-mini-2024-07-18").
	Model string
	// RequestID is the provider's per-call identifier, used by callers to
	// correlate this response with provider-side logs (support tickets,
	// audit trails, cross-system debugging). Adapters prefer the HTTP
	// response header when available and fall back to a body field.
	RequestID string
	// FinishReason is the provider's reason for ending generation.
	// Adapters pass the provider's native value through unchanged, so callers
	// should treat it as provider-specific rather than relying on one shared
	// set of finish-reason strings across all providers.
	FinishReason string
}

// NewClient creates a provider-specific AI client from provider-neutral
// configuration. It validates the config, then dispatches to the adapter
// registered for config.Provider. When config.DryRun is set, a logging-only
// client is returned that never contacts the provider.
func NewClient(config Config) (Client, error) {
	config = config.normalized()
	if err := config.validate(); err != nil {
		return nil, err
	}

	// The provider must be registered even in dry-run mode so typos surface
	// here instead of later when the user switches to a live run.
	factory, ok := clientFactories[config.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported ai provider %q", config.Provider)
	}

	// Dry-run short-circuits the factory: return a logging-only client
	// that never contacts the provider.
	if config.DryRun {
		return newDryRunClient(config), nil
	}

	return factory(config), nil
}

func (c Config) validate() error {
	if strings.TrimSpace(string(c.Provider)) == "" {
		return fmt.Errorf("ai provider is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("ai model is required")
	}
	// API key is only required for live provider calls. Dry-run mode lets
	// operators preview prompts without provisioning credentials.
	if !c.DryRun && strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("ai api key is required")
	}
	return nil
}

func (c Config) normalized() Config {
	c.Provider = Provider(strings.ToLower(strings.TrimSpace(string(c.Provider))))
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = defaultMaxTokens
	}
	return c
}

func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return &http.Client{Timeout: c.normalized().Timeout}
}
