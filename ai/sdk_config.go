package ai

import (
	"fmt"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// NewClientFromConfig is the convenience entrypoint for callers that
// already have an SDK config: it extracts the ai_* settings and, when
// they are present, builds and returns a ready-to-use Client. When AI
// is not configured it returns (nil, nil) so callers can treat AI as an
// optional capability without special-casing the "not set" path.
func NewClientFromConfig(config sdkconfig.Config) (Client, error) {
	aiConfig, configured, err := ConfigFromSDKConfig(config)
	if err != nil || !configured {
		return nil, err
	}

	return NewClient(aiConfig)
}

// ConfigFromSDKConfig extracts AI settings from the SDK config vars
// (ai_provider, ai_model, ai_api_key, ai_timeout, ai_max_tokens) into a
// provider-neutral Config. The configured return value is false only
// when none of the ai_* keys are set, letting callers distinguish
// "intentionally disabled" from "misconfigured" (which is returned as a
// non-nil error, e.g. an unparseable ai_timeout).
func ConfigFromSDKConfig(config sdkconfig.Config) (Config, bool, error) {
	provider := Provider(config.GetString("ai_provider"))
	model := config.GetString("ai_model")
	apiKey := config.GetString("ai_api_key")
	timeoutText := config.GetString("ai_timeout")
	maxTokens := config.GetInt("ai_max_tokens")

	if provider == "" && model == "" && apiKey == "" && timeoutText == "" && maxTokens == 0 {
		return Config{}, false, nil
	}

	aiConfig := Config{
		Provider:  provider,
		Model:     model,
		APIKey:    apiKey,
		MaxTokens: maxTokens,
	}

	if timeoutText != "" {
		timeout, err := time.ParseDuration(timeoutText)
		if err != nil {
			return Config{}, true, fmt.Errorf("invalid ai_timeout %q: %w", timeoutText, err)
		}
		aiConfig.Timeout = timeout
	}

	return aiConfig.normalized(), true, nil
}
