// Package ai is the single-import surface for the SDK's AI subsystem. It owns
// the registry of concrete provider adapters and re-exports the shared types
// so plugins depend on one package:
//
//   - ai/provider holds the provider-neutral contract (Client, Config, Schema,
//     AnalyzeResponse, Error) and the base adapters build on.
//   - ai/openai (and future sibling packages) hold concrete adapters,
//     registered in clientFactories below.
//   - ai/assist holds the plugin-facing accelerator surfaced here as Assist.
//
// See docs/ai-client.md and docs/ai-assist.md.
package ai

import (
	"context"
	"fmt"

	"github.com/gemaraproj/go-gemara"

	"github.com/privateerproj/privateer-sdk/ai/anthropic"
	"github.com/privateerproj/privateer-sdk/ai/assist"
	"github.com/privateerproj/privateer-sdk/ai/openai"
	"github.com/privateerproj/privateer-sdk/ai/provider"
	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// Aliases for the provider-neutral contract and the assist accelerator types,
// so callers import only this package.
type (
	Client           = provider.Client
	Config           = provider.Config
	Schema           = provider.Schema
	AnalyzeResponse  = provider.AnalyzeResponse
	ResponseMetadata = provider.ResponseMetadata
	Provider         = provider.Provider
	Error            = provider.Error
	ErrorKind        = provider.ErrorKind

	Question        = assist.Question
	Response        = assist.Response
	EvidencePayload = assist.EvidencePayload
)

const (
	ProviderOpenAI    = openai.Provider
	ProviderAnthropic = anthropic.Provider

	EvidenceType = assist.EvidenceType

	ErrorKindUnauthorized      = provider.ErrorKindUnauthorized
	ErrorKindRateLimited       = provider.ErrorKindRateLimited
	ErrorKindTimeout           = provider.ErrorKindTimeout
	ErrorKindProviderError     = provider.ErrorKindProviderError
	ErrorKindInvalidRequest    = provider.ErrorKindInvalidRequest
	ErrorKindInvalidResponse   = provider.ErrorKindInvalidResponse
	ErrorKindUnsupportedConfig = provider.ErrorKindUnsupportedConfig
)

// clientFactory builds a provider adapter from a normalized Config.
type clientFactory func(Config) Client

// clientFactories is the internal registry of built-in providers. Register a
// new adapter package's constructor here; calling code does not change.
var clientFactories = map[Provider]clientFactory{
	openai.Provider: func(config Config) Client {
		return openai.NewClient(config)
	},
	anthropic.Provider: func(config Config) Client {
		return anthropic.NewClient(config)
	},
}

// NewClient is the default constructor for plugins: it extracts the ai_*
// settings from the primary end user config and builds a Client from them. When
// AI is not configured it returns (nil, nil) — check the returned client, not
// just the error, and treat a nil client as "AI disabled".
func NewClient(config sdkconfig.Config) (Client, error) {
	aiConfig, configured, err := provider.ConfigFromSDKConfig(config)
	if err != nil || !configured {
		return nil, err
	}

	return NewClientWithAIConfig(aiConfig)
}

// NewClientWithAIConfig builds a client from an explicit provider-neutral Config.
// It validates the config and dispatches to the adapter registered for
// config.Provider.
func NewClientWithAIConfig(config Config) (Client, error) {
	config = config.Normalized()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	factory, ok := clientFactories[config.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported ai provider %q", config.Provider)
	}

	return factory(config), nil
}

// Assist runs an AI-assisted assessment via the assist package: it asks client
// for a structured Response answering q, then packages that verdict as a
// gemara.Evidence. See assist.Assist.
func Assist(ctx context.Context, client Client, q Question) (Response, gemara.Evidence, error) {
	return assist.Assist(ctx, client, q)
}
