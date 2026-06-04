package ai

import (
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
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
// (ai_provider, ai_model, ai_api_key, ai_base_url, ai_timeout, ai_max_tokens,
// ai_dry_run) and the --dry-run-ai CLI flag into a provider-neutral Config. The
// configured return value is false only when none of these inputs are set,
// letting callers distinguish "intentionally disabled" from "misconfigured"
// (which is returned as a non-nil error, e.g. an unparseable ai_timeout).
func ConfigFromSDKConfig(config sdkconfig.Config) (Config, bool, error) {
	provider := Provider(getSDKConfigString(config, "ai_provider"))
	model := getSDKConfigString(config, "ai_model")
	apiKey := getSDKConfigString(config, "ai_api_key")
	baseURL := getSDKConfigString(config, "ai_base_url")
	timeoutText := getSDKConfigString(config, "ai_timeout")
	maxTokens := getSDKConfigInt(config, "ai_max_tokens")
	// Dry-run is enabled by either the config-driven key (for non-CLI
	// consumers and YAML configs) or the CLI flag (bound to viper in
	// command.SetBase). Both sources are checked so either path works.
	dryRun := getSDKConfigBool(config, "ai_dry_run") || viper.GetBool("dry-run-ai")

	if provider == "" && model == "" && apiKey == "" && baseURL == "" && timeoutText == "" && maxTokens == 0 && !dryRun {
		return Config{}, false, nil
	}

	aiConfig := Config{
		Provider:  provider,
		Model:     model,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		MaxTokens: maxTokens,
		DryRun:    dryRun,
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

func getSDKConfigString(config sdkconfig.Config, key string) string {
	if value := strings.TrimSpace(config.GetString(key)); value != "" {
		return value
	}
	return strings.TrimSpace(viper.GetString(key))
}

func getSDKConfigInt(config sdkconfig.Config, key string) int {
	if value := config.GetInt(key); value != 0 {
		return value
	}
	return viper.GetInt(key)
}

func getSDKConfigBool(config sdkconfig.Config, key string) bool {
	if value := config.GetBool(key); value {
		return true
	}
	return viper.GetBool(key)
}
