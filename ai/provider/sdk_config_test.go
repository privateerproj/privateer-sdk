package provider

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
	if aiConfig.Provider != testProvider {
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
	if aiConfig.Provider != testProvider {
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

func TestConfigFromSDKConfig_RejectsWrongTypedVars(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]interface{}
		wantErrText string
	}{
		{
			name: "string field with int value",
			vars: map[string]interface{}{
				"ai_provider": 123,
			},
			wantErrText: "ai_provider must be a string, got int",
		},
		{
			name: "int field with string value",
			vars: map[string]interface{}{
				"ai_provider":   "openai",
				"ai_model":      "gpt-4o-mini",
				"ai_api_key":    "test-key",
				"ai_max_tokens": "512",
			},
			wantErrText: "ai_max_tokens must be an int, got string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			_, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: tt.vars})
			if !configured {
				t.Fatal("expected configured result for wrong-typed AI config")
			}
			if err == nil {
				t.Fatal("expected wrong-type error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrText)
			}
		})
	}
}

func TestGetSDKConfigString_ResolvesOnPresence(t *testing.T) {
	t.Run("explicit empty string in config Vars is honored over viper", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_base_url", "http://127.0.0.1:8000/v1")

		config := sdkconfig.Config{Vars: map[string]interface{}{"ai_base_url": ""}}
		got, err := getSDKConfigString(config, "ai_base_url")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected explicit empty string from config Vars, got %q", got)
		}
	})

	t.Run("falls through to viper when absent from config Vars", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_base_url", "http://127.0.0.1:8000/v1")

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		got, err := getSDKConfigString(config, "ai_base_url")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "http://127.0.0.1:8000/v1" {
			t.Fatalf("expected viper fallback base URL, got %q", got)
		}
	})
}

func TestGetSDKConfigInt_ResolvesOnPresence(t *testing.T) {
	t.Run("explicit zero in config Vars is honored over viper", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 5)

		config := sdkconfig.Config{Vars: map[string]interface{}{"ai_retries": 0}}
		got, err := getSDKConfigInt(config, "ai_retries")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Fatalf("expected explicit 0 from config Vars, got %d", got)
		}
	})

	t.Run("falls through to viper when absent from config Vars", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 5)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		got, err := getSDKConfigInt(config, "ai_retries")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 5 {
			t.Fatalf("expected viper fallback 5, got %d", got)
		}
	})

	t.Run("explicit zero in viper is honored when set", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("ai_retries", 0)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		got, err := getSDKConfigInt(config, "ai_retries")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Fatalf("expected explicit viper 0, got %d", got)
		}
	})

	t.Run("returns 0 when unset everywhere", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		config := sdkconfig.Config{Vars: map[string]interface{}{}}
		got, err := getSDKConfigInt(config, "ai_retries")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Fatalf("expected 0 when unset, got %d", got)
		}
	})
}
