package ai

import (
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

// ConfigFromSDKConfig extracts the ai_* settings into a provider-neutral
// Config. The configured return is false only when none are set,
// distinguishing "intentionally disabled" from "misconfigured" (a non-nil
// error, e.g. an unparseable ai_timeout).
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
	if provider == "" && model == "" && apiKey == "" && baseURL == "" && timeoutText == "" && maxTokens == 0 {
		return Config{}, false, nil
	}

	aiConfig := Config{
		Provider:  provider,
		Model:     model,
		APIKey:    apiKey,
		BaseURL:   baseURL,
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

// getSDKConfigString resolves on key presence, not value, so an explicit empty
// string in per-service Vars is honored over viper. A non-string value is an error.
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
// per-service Vars is honored over viper. A non-int value is an error.
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
