package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

// ConfigFromSDKConfig extracts enabled ai_* settings into a provider-neutral
// Config. The returned bool reports whether AI is enabled; when it is false the
// returned Config is zero and callers must skip AI entirely.
func ConfigFromSDKConfig(config sdkconfig.Config) (Config, bool, error) {
	skip, err := resolveAISkip(config)
	if err != nil {
		return Config{}, true, err
	}
	if skip {
		return Config{}, false, nil
	}

	// ai_provider is the only key that can enable AI: unset means AI is off, and
	// no other ai_* key turns it on.
	providerText, err := getSDKConfigString(config, "ai_provider")
	if err != nil {
		return Config{}, true, err
	}
	if providerText == "" {
		return Config{}, false, nil
	}

	provider := Provider(providerText)
	model, err := getSDKConfigString(config, "ai_model")
	if err != nil {
		return Config{}, true, err
	}
	apiKey, err := resolveAPIKey(config)
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
		if timeout <= 0 {
			return Config{}, true, fmt.Errorf("ai_timeout must be positive, got %q", timeoutText)
		}
		aiConfig.Timeout = timeout
	}
	if isSDKConfigKeySet(config, "ai_max_tokens") && maxTokens <= 0 {
		return Config{}, true, fmt.Errorf("ai_max_tokens must be positive, got %d", maxTokens)
	}

	return aiConfig.Normalized(), true, nil
}

// resolveAPIKey applies the credential precedence, highest first: target
// ai_api_key, target ai_api_key_env, PVTR_AI_API_KEY, top-level ai_api_key,
// then top-level ai_api_key_env. The viper lookup preserves compatibility for
// callers that configure viper directly instead of calling config.NewConfig.
func resolveAPIKey(config sdkconfig.Config) (string, error) {
	targetVars, targetConfigured := sdkconfig.GetTargetVars(config.ServiceName)
	if !targetConfigured {
		targetVars = config.Vars
	}
	if len(targetVars) > 0 {
		if apiKey, found, err := stringValue(targetVars, "ai_api_key"); err != nil {
			return "", err
		} else if found && apiKey != "" {
			return apiKey, nil
		}
		if envName, found, err := stringValue(targetVars, "ai_api_key_env"); found || err != nil {
			if err != nil {
				return "", err
			}
			return apiKeyFromEnv(envName)
		}
	}

	if apiKey := strings.TrimSpace(os.Getenv("PVTR_AI_API_KEY")); apiKey != "" {
		return apiKey, nil
	}
	if apiKey, found, err := stringValue(config.Vars, "ai_api_key"); err != nil {
		return "", err
	} else if found && apiKey != "" {
		return apiKey, nil
	}
	if apiKey := strings.TrimSpace(viper.GetString("ai_api_key")); apiKey != "" {
		return apiKey, nil
	}
	if envName, found, err := stringValue(config.Vars, "ai_api_key_env"); found || err != nil {
		if err != nil {
			return "", err
		}
		return apiKeyFromEnv(envName)
	}

	return "", nil
}

func stringValue(vars map[string]interface{}, key string) (string, bool, error) {
	value, found := vars[key]
	if !found {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("%s must be a string, got %T", key, value)
	}
	return strings.TrimSpace(text), true, nil
}

func apiKeyFromEnv(envName string) (string, error) {
	if envName == "" {
		return "", fmt.Errorf("ai_api_key_env must name a non-empty environment variable")
	}
	apiKey := strings.TrimSpace(os.Getenv(envName))
	if apiKey == "" {
		return "", fmt.Errorf("ai_api_key_env names environment variable %q, but it is unset or empty", envName)
	}
	return apiKey, nil
}

// getSDKConfigString resolves on key presence, not value, so an explicit empty
// string in per-service Vars is honored over viper. A non-string value is an error.
func getSDKConfigString(config sdkconfig.Config, key string) (string, error) {
	if value, found := sdkEnvironmentString(key); found {
		return value, nil
	}
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
	if text, found := sdkEnvironmentString(key); found {
		value, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("%s must be an int, got environment value %q", key, text)
		}
		return value, nil
	}
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

func resolveAISkip(config sdkconfig.Config) (bool, error) {
	var invalidType string
	text, environmentSet := sdkEnvironmentString("ai_skip")
	if environmentSet {
		skip, err := strconv.ParseBool(text)
		if err == nil && skip {
			return true, nil
		}
		if err != nil {
			invalidType = fmt.Sprintf("ai_skip must be a bool, got environment value %q", text)
		}
	}
	if value, valType := config.GetVar("ai_skip"); valType == "bool" {
		if value.(bool) {
			return true, nil
		}
	} else if valType != "missing" && invalidType == "" {
		invalidType = fmt.Sprintf("ai_skip must be a bool, got %s", valType)
	}
	if viper.IsSet("ai_skip") {
		value := viper.Get("ai_skip")
		valueText, duplicatesEnvironment := value.(string)
		duplicatesEnvironment = environmentSet && duplicatesEnvironment && strings.TrimSpace(valueText) == text
		if skip, ok := value.(bool); ok {
			if skip {
				return true, nil
			}
		} else if !duplicatesEnvironment && invalidType == "" {
			invalidType = fmt.Sprintf("ai_skip must be a bool, got %T", value)
		}
	}
	if invalidType != "" {
		return false, fmt.Errorf("%s", invalidType)
	}
	return false, nil
}

func sdkEnvironmentString(key string) (string, bool) {
	value, found := os.LookupEnv("PVTR_" + strings.ToUpper(key))
	value = strings.TrimSpace(value)
	return value, found && value != ""
}

func isSDKConfigKeySet(config sdkconfig.Config, key string) bool {
	if _, found := sdkEnvironmentString(key); found {
		return true
	}
	_, valType := config.GetVar(key)
	return valType != "missing" || viper.IsSet(key)
}
