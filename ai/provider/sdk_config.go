package provider

import (
	"fmt"
	"os"
	"strings"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// ConfigFromSDKConfig extracts enabled ai_* settings into a provider-neutral
// Config. The bool reports whether AI is enabled; it is false whenever the
// error is non-nil, so callers must check the error before treating false as
// "AI disabled".
//
// A true result means AI was asked for and the settings that were present
// parsed, not that the set is complete: a config naming a provider but no
// model returns true with a nil error. Completeness is Config.Validate's job,
// which ai.NewClientWithAIConfig runs before constructing an adapter, so a
// caller using this function directly must call Validate itself.
func ConfigFromSDKConfig(config sdkconfig.Config) (Config, bool, error) {
	if config.Error != nil {
		return Config{}, false, config.Error
	}
	if config.GetBool("ai_skip") {
		return Config{}, false, nil
	}

	// ai_provider is the only key that can enable AI: unset means AI is off, and
	// no other ai_* key turns it on.
	providerText, err := getSDKConfigString(config, "ai_provider")
	if err != nil {
		return Config{}, false, err
	}
	if providerText == "" {
		return Config{}, false, nil
	}
	if _, valType := config.GetVar("ai_skip"); valType != "missing" && valType != "bool" {
		return Config{}, false, fmt.Errorf("ai_skip must be a bool, got %s", valType)
	}

	provider := Provider(providerText)
	model, err := getSDKConfigString(config, "ai_model")
	if err != nil {
		return Config{}, false, err
	}
	apiKey, err := resolveAPIKey(config)
	if err != nil {
		return Config{}, false, err
	}
	baseURL, err := getSDKConfigString(config, "ai_base_url")
	if err != nil {
		return Config{}, false, err
	}
	timeoutText, err := getSDKConfigString(config, "ai_timeout")
	if err != nil {
		return Config{}, false, err
	}
	maxTokens, err := getSDKConfigInt(config, "ai_max_tokens")
	if err != nil {
		return Config{}, false, err
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
			return Config{}, false, fmt.Errorf("invalid ai_timeout %q: %w", timeoutText, err)
		}
		if timeout <= 0 {
			return Config{}, false, fmt.Errorf("ai_timeout must be positive, got %q", timeoutText)
		}
		aiConfig.Timeout = timeout
	}
	if isSDKConfigKeySet(config, "ai_max_tokens") && maxTokens <= 0 {
		return Config{}, false, fmt.Errorf("ai_max_tokens must be positive, got %d", maxTokens)
	}

	return aiConfig.Normalized(), true, nil
}

// NewConfig selects the credential source. Hand-built Vars may also explicitly
// name an environment variable; no ambient environment or Viper settings apply.
func resolveAPIKey(config sdkconfig.Config) (string, error) {
	if envName, found, err := stringValue(config.Vars, "ai_api_key_env"); found || err != nil {
		if err != nil {
			return "", err
		}
		return apiKeyFromEnv(envName)
	}
	return getSDKConfigString(config, "ai_api_key")
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

// aiAPIKeyEnvPrefix bounds ai_api_key_env to Privateer's own environment
// namespace. Every other setting reaches the environment through Viper, which
// is pinned to the PVTR_ prefix, so a configuration file can only ever read
// variables this tool owns. ai_api_key_env dereferences a caller-supplied name
// directly, which escapes that boundary: without this bound, a config.yml
// picked up from the working directory could name GITHUB_TOKEN or
// AWS_SECRET_ACCESS_KEY and pair it with an ai_base_url of its choosing.
//
// The bound is enforced here rather than during config resolution because this
// is the single point where a variable name becomes a credential, so direct
// SDK callers that build Config.Vars themselves are covered too.
const aiAPIKeyEnvPrefix = "PVTR_AI_"

func apiKeyFromEnv(envName string) (string, error) {
	if envName == "" {
		return "", fmt.Errorf("ai_api_key_env must name a non-empty environment variable")
	}
	if !strings.HasPrefix(envName, aiAPIKeyEnvPrefix) {
		return "", fmt.Errorf("ai_api_key_env names environment variable %q, but it must begin with %s so that a configuration file cannot read credentials outside Privateer's namespace: export the credential as %sAPI_KEY, or under another %s name for a per-target credential", envName, aiAPIKeyEnvPrefix, aiAPIKeyEnvPrefix, aiAPIKeyEnvPrefix)
	}
	apiKey := strings.TrimSpace(os.Getenv(envName))
	if apiKey == "" {
		return "", fmt.Errorf("ai_api_key_env names environment variable %q, but it is unset or empty", envName)
	}
	return apiKey, nil
}

// getSDKConfigString reads the resolved Vars without consulting global state.
func getSDKConfigString(config sdkconfig.Config, key string) (string, error) {
	value, valType := config.GetVar(key)
	switch valType {
	case "missing":
		return "", nil
	case "string":
		return strings.TrimSpace(value.(string)), nil
	default:
		return "", fmt.Errorf("%s must be a string, got %s", key, valType)
	}
}

// getSDKConfigInt preserves explicit zero values for validation by the caller.
func getSDKConfigInt(config sdkconfig.Config, key string) (int, error) {
	value, valType := config.GetVar(key)
	switch valType {
	case "missing":
		return 0, nil
	case "int":
		return value.(int), nil
	default:
		return 0, fmt.Errorf("%s must be an int, got %s", key, valType)
	}
}

func isSDKConfigKeySet(config sdkconfig.Config, key string) bool {
	_, valType := config.GetVar(key)
	return valType != "missing"
}
