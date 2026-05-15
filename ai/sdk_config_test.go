package ai

import (
	"testing"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

func TestConfigFromSDKConfig_NotConfigured(t *testing.T) {
	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configured {
		t.Fatalf("expected unconfigured result, got configured with %#v", aiConfig)
	}
}

func TestConfigFromSDKConfig_DefaultsAndParsing(t *testing.T) {
	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
		"ai_timeout":  "45s",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result")
	}
	if aiConfig.Provider != ProviderOpenAI {
		t.Fatalf("unexpected provider: %s", aiConfig.Provider)
	}
	if aiConfig.Timeout != 45*time.Second {
		t.Fatalf("unexpected timeout: %s", aiConfig.Timeout)
	}
	if aiConfig.MaxTokens != defaultMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", defaultMaxTokens, aiConfig.MaxTokens)
	}
}

func TestConfigFromSDKConfig_InvalidTimeout(t *testing.T) {
	_, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
		"ai_timeout":  "bad-timeout",
	}})
	if !configured {
		t.Fatal("expected configured result for partial AI config")
	}
	if err == nil {
		t.Fatal("expected invalid timeout error, got nil")
	}
}

func TestNewClientFromConfig(t *testing.T) {
	client, err := NewClientFromConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected configured client, got nil")
	}
	if _, ok := client.(*openAIClient); !ok {
		t.Fatalf("expected *openAIClient, got %T", client)
	}
}
