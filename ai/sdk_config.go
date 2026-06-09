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
	providerText, err := getSDKConfigString(config, "ai_provider")
	if err != nil {
		return Config{}, true, err
	}
	provider := Provider(providerText)
	model, err := getSDKConfigString(config, "ai_model")
	if err != nil {
		return Config{}, true, err
	}
	apiKey, err := getSDKConfigString(config, "ai_api_key")
	if err != nil {
		return Config{}, true, err
	}
	baseURL, err := getSDKConfigString(config, "ai_base_url")
	if err != nil {
		return Config{}, true, err
	}
	timeoutText, err := getSDKConfigString(config, "ai_timeout")
	if err != nil {
		return Config{}, true, err
	}
	maxTokens, err := getSDKConfigInt(config, "ai_max_tokens")
	if err != nil {
		return Config{}, true, err
	}
	// ai_dry_run is one logical key fed by config Vars, top-level YAML/env, and
	// the --dry-run-ai CLI flag (bound to ai_dry_run in command.SetBase).
	dryRun, err := getSDKConfigBool(config, "ai_dry_run")
	if err != nil {
		return Config{}, true, err
	}

	// Dry-run by itself is treated as AI configuration so provider/model
	// validation fails early instead of being deferred to first use.
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

// getSDKConfigString resolves on key presence, not value, so an explicit empty
// string in per-service Vars is honored instead of falling through to viper.
// A present non-string value is treated as misconfigured AI input.
func getSDKConfigString(config sdkconfig.Config, key string) (string, error) {
	value, valType := config.GetVar(key)
	switch valType {
	case "missing":
		return strings.TrimSpace(viper.GetString(key)), nil
	case "string":
		return strings.TrimSpace(value.(string)), nil
	default:
		return "", fmt.Errorf("%s must be a string, got %s", key, valType)
	}
}

// getSDKConfigInt resolves on key presence, not value, so an explicit 0 in
// per-service Vars is honored instead of falling through to viper. A present
// non-int value is treated as misconfigured AI input.
func getSDKConfigInt(config sdkconfig.Config, key string) (int, error) {
	value, valType := config.GetVar(key)
	switch valType {
	case "missing":
		if viper.IsSet(key) {
			return viper.GetInt(key), nil
		}
		return 0, nil
	case "int":
		return value.(int), nil
	default:
		return 0, fmt.Errorf("%s must be an int, got %s", key, valType)
	}
}

// getSDKConfigBool resolves on key presence, not value, so an explicit false in
// per-service Vars is honored instead of falling through to viper. A present
// non-bool value is treated as misconfigured AI input.
func getSDKConfigBool(config sdkconfig.Config, key string) (bool, error) {
	value, valType := config.GetVar(key)
	switch valType {
	case "missing":
		return viper.GetBool(key), nil
	case "bool":
		return value.(bool), nil
	default:
		return false, fmt.Errorf("%s must be a bool, got %s", key, valType)
	}
}
