// Package provider defines the provider-neutral contract callers use to talk
// to any AI backend, plus the shared base every concrete adapter builds on.
// Concrete adapters live in sibling packages (e.g. ai/openai) and are wired
// into the registry in the parent ai package. See docs/ai-client.md.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Provider identifies an AI backend implementation. Each adapter package
// exports its own constant (e.g. openai.Provider) and is registered in the
// parent ai package's factory registry.
type Provider string

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 256
)

// Client is the provider-neutral analysis contract every adapter satisfies.
type Client interface {
	Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error)
}

// Config holds the provider-neutral settings used to construct any adapter.
// See docs/ai-client.md for field semantics and defaults.
type Config struct {
	// Provider selects which adapter is constructed.
	Provider Provider
	// APIKey is the credential passed to the provider.
	APIKey string
	// Model is the provider-specific model identifier (e.g. "gpt-4o-mini").
	Model string
	// BaseURL overrides the adapter's default endpoint. Empty uses the default.
	BaseURL string
	// Timeout bounds a single Analyze call. Zero falls back to defaultTimeout.
	Timeout time.Duration
	// MaxTokens caps the response length. Zero falls back to defaultMaxTokens.
	MaxTokens int
	// HTTPClient injects a custom transport. Nil builds a Timeout-honoring client.
	HTTPClient *http.Client
}

// Schema tells the provider to return a structured JSON answer instead of
// free-form text. The adapter translates the JSON Schema document into whatever
// the provider expects; when supplied, AnalyzeResponse.JSON holds the parsed
// payload. See docs/ai-client.md for a worked example.
type Schema struct {
	// Name labels the schema for the provider. Required when Schema is supplied.
	Name string
	// Description tells the model what the schema is for. Optional.
	Description string
	// Value is the JSON Schema document the response must conform to.
	Value json.RawMessage
	// Strict asks the provider to reject responses that do not match Value
	// exactly. Providers without strict mode ignore it.
	Strict bool
}

// AnalyzeResponse is the normalized result returned by every adapter. Without a
// Schema only Text is populated; with a Schema, JSON holds the parsed payload
// and Text holds the raw message content.
type AnalyzeResponse struct {
	// Text is the raw text content of the model's response.
	Text string
	// JSON is the parsed structured payload, non-nil only when a Schema was
	// supplied and the provider produced valid JSON.
	JSON json.RawMessage
	// Metadata describes the response in provider-neutral terms.
	Metadata ResponseMetadata
}

// ResponseMetadata is diagnostic metadata about an Analyze call. The SDK does
// not read these fields itself; they are returned for logging, audit trails,
// attribution, or branching on FinishReason. See docs/ai-client.md.
type ResponseMetadata struct {
	// Provider is the adapter that produced the response.
	Provider Provider
	// Model is the model the provider reports having used, which may differ
	// from Config.Model when the requested name is a resolved alias.
	Model string
	// RequestID is the provider's per-call id for correlating with
	// provider-side logs.
	RequestID string
	// FinishReason is the provider's native reason for ending generation,
	// passed through unchanged (treat as provider-specific).
	FinishReason string
}

// Validate reports whether the config carries the fields every adapter needs.
// Call it on a Normalized config; the parent ai package does so before
// dispatching to an adapter factory.
func (c Config) Validate() error {
	if strings.TrimSpace(string(c.Provider)) == "" {
		return fmt.Errorf("ai provider is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("ai model is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("ai api key is required")
	}
	return nil
}

// Normalized returns a copy with fields trimmed, the provider lowercased, and
// zero Timeout/MaxTokens replaced by package defaults.
func (c Config) Normalized() Config {
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

// httpClient is called on an already-Normalized config (NewBase normalizes),
// so Timeout is guaranteed non-zero here.
func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return &http.Client{Timeout: c.Timeout}
}

// String renders the config with the APIKey redacted, so accidentally logging
// a config (or an adapter embedding one) cannot leak the credential.
func (c Config) String() string {
	apiKey := "<unset>"
	if c.APIKey != "" {
		apiKey = "<redacted>"
	}
	return fmt.Sprintf("{Provider:%s APIKey:%s Model:%s BaseURL:%s Timeout:%v MaxTokens:%d HTTPClient:%t}",
		c.Provider, apiKey, c.Model, c.BaseURL, c.Timeout, c.MaxTokens, c.HTTPClient != nil)
}
