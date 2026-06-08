package ai

import (
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
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
		"ai_base_url": "http://127.0.0.1:8000/v1",
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
	if aiConfig.BaseURL != "http://127.0.0.1:8000/v1" {
		t.Fatalf("unexpected base URL: %q", aiConfig.BaseURL)
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

func TestNewClientFromConfig_PartialLiveConfigErrors(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]interface{}
		wantErrText string
	}{
		{
			name: "provider only",
			vars: map[string]interface{}{
				"ai_provider": "openai",
			},
			wantErrText: "ai model is required",
		},
		{
			name: "model only",
			vars: map[string]interface{}{
				"ai_model": "gpt-4o-mini",
			},
			wantErrText: "ai provider is required",
		},
		{
			name: "provider and model without api key",
			vars: map[string]interface{}{
				"ai_provider": "openai",
				"ai_model":    "gpt-4o-mini",
			},
			wantErrText: "ai api key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			client, err := NewClientFromConfig(sdkconfig.Config{Vars: tt.vars})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrText)
			}
			if client != nil {
				t.Fatalf("expected nil client, got %T", client)
			}
		})
	}
}

func TestConfigFromSDKConfig_DryRunViaConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_dry_run":  true,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result")
	}
	if !aiConfig.DryRun {
		t.Fatal("expected DryRun to be true")
	}
}

func TestConfigFromSDKConfig_DryRunViaCLIFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	// The --dry-run-ai flag binds to the ai_dry_run viper key in
	// command.SetBase (verified in command/base_test.go); here we set that
	// resolved key directly to confirm ConfigFromSDKConfig reads it.
	viper.Set("ai_dry_run", true)

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result")
	}
	if !aiConfig.DryRun {
		t.Fatal("expected DryRun to be true from CLI flag")
	}
}

func TestConfigFromSDKConfig_DryRunViaEnv(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_DRY_RUN", "true")
	viper.SetEnvPrefix("PVTR")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result")
	}
	if !aiConfig.DryRun {
		t.Fatal("expected DryRun to be true from PVTR_AI_DRY_RUN")
	}
}

func TestNewClientFromConfig_DryRunSkipsAPIKey(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	client, err := NewClientFromConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_dry_run":  true,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*dryRunClient); !ok {
		t.Fatalf("expected *dryRunClient, got %T", client)
	}
}

func TestConfigFromSDKConfig_DryRunDefaultsFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result")
	}
	if aiConfig.DryRun {
		t.Fatal("expected DryRun to default to false when neither config var nor flag is set")
	}
}

func TestConfigFromSDKConfig_UsesViperFallback(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("ai_provider", "openai")
	viper.Set("ai_model", "gpt-4o-mini")
	viper.Set("ai_api_key", "test-key")
	viper.Set("ai_timeout", "45s")
	viper.Set("ai_max_tokens", 512)

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured result from viper-backed settings")
	}
	if aiConfig.Provider != ProviderOpenAI {
		t.Fatalf("unexpected provider: %s", aiConfig.Provider)
	}
	if aiConfig.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %s", aiConfig.Model)
	}
	if aiConfig.APIKey != "test-key" {
		t.Fatalf("unexpected api key: %s", aiConfig.APIKey)
	}
	if aiConfig.Timeout != 45*time.Second {
		t.Fatalf("unexpected timeout: %s", aiConfig.Timeout)
	}
	if aiConfig.MaxTokens != 512 {
		t.Fatalf("unexpected max tokens: %d", aiConfig.MaxTokens)
	}
}

func TestGetSDKConfigInt_ResolvesOnPresence(t *testing.T) {
	t.Run("explicit zero in config Vars is honored over viper", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 5)

		config := sdkconfig.Config{Vars: map[string]interface{}{"ai_retries": 0}}
		if got := getSDKConfigInt(config, "ai_retries"); got != 0 {
			t.Fatalf("expected explicit 0 from config Vars, got %d", got)
		}
	})

	t.Run("falls through to viper when absent from config Vars", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 5)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		if got := getSDKConfigInt(config, "ai_retries"); got != 5 {
			t.Fatalf("expected viper fallback 5, got %d", got)
		}
	})

	t.Run("explicit zero in viper is honored when set", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 0)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		if got := getSDKConfigInt(config, "ai_retries"); got != 0 {
			t.Fatalf("expected explicit viper 0, got %d", got)
		}
	})

	t.Run("returns 0 when unset everywhere", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		if got := getSDKConfigInt(config, "ai_retries"); got != 0 {
			t.Fatalf("expected 0 when unset, got %d", got)
		}
	})
}
