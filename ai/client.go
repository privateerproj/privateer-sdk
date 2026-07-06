// Package ai defines the provider-neutral contract callers use to talk to any
// AI backend. Concrete providers are adapters registered in clientFactories;
// callers use only the shared types. See docs/ai-client.md.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// Provider identifies the AI backend implementation. Each has a constant and a
// matching entry in clientFactories.
type Provider string

const (
	ProviderOpenAI Provider = "openai"
	// Add new providers here alongside a clientFactories entry.
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 256
)

// clientFactory builds a provider adapter from a normalized Config.
type clientFactory func(Config) Client

// clientFactories is the internal registry of built-in providers. Register a
// new adapter's constructor here; calling code does not change.
var clientFactories = map[Provider]clientFactory{
	ProviderOpenAI: func(config Config) Client {
		return newOpenAIClient(config)
	},
}

// Client is the provider-neutral analysis contract every adapter satisfies.
type Client interface {
	Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error)
}

// Config holds the provider-neutral settings used to construct any adapter.
// See docs/ai-client.md for field semantics and defaults.
type Config struct {
	// Provider selects which adapter is constructed via clientFactories.
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

// NewClient is the default constructor for plugins: it extracts the ai_*
// settings from the primary end user config and builds a Client from them. When
// AI is not configured it returns (nil, nil) — check the returned client, not
// just the error, and treat a nil client as "AI disabled".
func NewClient(config sdkconfig.Config) (Client, error) {
	aiConfig, configured, err := ConfigFromSDKConfig(config)
	if err != nil || !configured {
		return nil, err
	}

	return NewClientWithConfig(aiConfig)
}

// NewClientWithConfig builds a client from an explicit provider-neutral Config.
// It validates the config and dispatches to the adapter registered for
// config.Provider.
func NewClientWithConfig(config Config) (Client, error) {
	config = config.normalized()
	if err := config.validate(); err != nil {
		return nil, err
	}

	factory, ok := clientFactories[config.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported ai provider %q", config.Provider)
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
	if strings.TrimSpace(c.APIKey) == "" {
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
