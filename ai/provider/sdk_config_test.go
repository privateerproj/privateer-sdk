package provider

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

func configFromFileForTarget(t *testing.T, content, target string) sdkconfig.Config {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	configFile := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig() error = %v", err)
	}
	viper.SetEnvPrefix("PVTR")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.Set("service", target)
	return sdkconfig.NewConfig(nil)
}

func TestConfigFromSDKConfig_NotConfigured(t *testing.T) {
	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configured {
		t.Fatalf("expected unconfigured result, got configured with %#v", aiConfig)
	}
}

func TestConfigFromSDKConfig_StrayNonProviderKeysDoNotEnableAI(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ai_timeout", "45s")

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_model":      "gpt-4o-mini",
		"ai_max_tokens": 512,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configured {
		t.Fatalf("expected disabled AI, got %#v", aiConfig)
	}
}

func TestConfigFromSDKConfig_AISkip(t *testing.T) {
	tests := []struct {
		name string
		// config drives the file pipeline; vars builds the Config by hand.
		// viperSkip seeds ai_skip directly in Viper for the hand-built cases.
		config    string
		vars      map[string]interface{}
		viperSkip interface{}
		envSkip   string
		wantErr   string
	}{
		{
			name: "target opts out",
			vars: map[string]interface{}{"ai_provider": "openai", "ai_skip": true},
		},
		{
			name:      "target false cannot override top-level true",
			vars:      map[string]interface{}{"ai_provider": "openai", "ai_skip": false},
			viperSkip: true,
		},
		{
			name:      "environment false cannot override direct Viper true",
			vars:      map[string]interface{}{"ai_provider": "openai"},
			viperSkip: true,
			envSkip:   "false",
		},
		{
			name: "top-level true beats environment false",
			config: `
ai_provider: 123
ai_skip: true
services:
  repo-one:
    policy: {catalogs: [catalog], applicability: [all]}
`,
			envSkip: "false",
		},
		{
			name: "target true beats environment false",
			config: `
services:
  repo-one:
    vars: {ai_provider: 123, ai_skip: true}
    policy: {catalogs: [catalog], applicability: [all]}
`,
			envSkip: "false",
		},
		{
			name: "top-level true suppresses malformed target skip",
			config: `
ai_skip: true
services:
  repo-one:
    vars: {ai_provider: 123, ai_skip: not-a-bool}
    policy: {catalogs: [catalog], applicability: [all]}
`,
		},
		{
			name: "environment true suppresses malformed target config",
			config: `
services:
  repo-one:
    vars: {ai_provider: 123, ai_skip: not-a-bool}
    policy: {catalogs: [catalog], applicability: [all]}
`,
			envSkip: "true",
		},
		{
			name: "target opts out of inherited global AI",
			config: `
ai_provider: openai
ai_model: top-model
ai_base_url: https://gateway.example/v1
services:
  repo-one:
    vars: {ai_skip: true}
    policy: {catalogs: [catalog], applicability: [all]}
`,
		},
		{
			// The ambient PVTR_AI_SKIP no longer participates for a hand-built
			// Config, so the target's own type error is what surfaces.
			name:    "wrong-typed target value",
			vars:    map[string]interface{}{"ai_provider": "openai", "ai_skip": 123},
			envSkip: "not-a-bool",
			wantErr: "ai_skip must be a bool, got int",
		},
		{
			name: "environment false does not hide malformed target value",
			config: `
services:
  repo-one:
    vars: {ai_provider: openai, ai_model: model, ai_api_key: key, ai_skip: not-a-bool}
    policy: {catalogs: [catalog], applicability: [all]}
`,
			envSkip: "false",
			wantErr: "ai_skip must be a bool",
		},
		{
			name: "environment false does not hide malformed top-level value",
			config: `
ai_skip: not-a-bool
services:
  repo-one:
    vars: {ai_provider: openai, ai_model: model, ai_api_key: key}
    policy: {catalogs: [catalog], applicability: [all]}
`,
			envSkip: "false",
			wantErr: "ai_skip must be a bool",
		},
		{
			name:      "wrong-typed Viper fallback",
			vars:      map[string]interface{}{"ai_provider": "openai"},
			viperSkip: 123,
			wantErr:   "ai_skip must be a bool, got int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PVTR_AI_SKIP", tt.envSkip)

			var cfg sdkconfig.Config
			if tt.config != "" {
				cfg = configFromFileForTarget(t, tt.config, "repo-one")
			} else {
				viper.Reset()
				t.Cleanup(viper.Reset)
				if tt.viperSkip != nil {
					viper.Set("ai_skip", tt.viperSkip)
				}
				cfg = sdkconfig.Config{Vars: tt.vars}
			}

			aiConfig, configured, err := ConfigFromSDKConfig(cfg)
			if configured {
				t.Fatalf("expected disabled AI, got %#v", aiConfig)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ConfigFromSDKConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigFromSDKConfig_EnvironmentOnly(t *testing.T) {
	t.Setenv("PVTR_AI_PROVIDER", "openai")
	t.Setenv("PVTR_AI_MODEL", "env-model")
	t.Setenv("PVTR_AI_API_KEY", "env-credential")
	t.Setenv("PVTR_AI_BASE_URL", "https://gateway.example/v1")
	t.Setenv("PVTR_AI_TIMEOUT", "75s")
	t.Setenv("PVTR_AI_MAX_TOKENS", "2048")
	cfg := configFromFileForTarget(t, `
services:
  repo-one:
    policy: {catalogs: [catalog], applicability: [all]}
`, "repo-one")

	aiConfig, configured, err := ConfigFromSDKConfig(cfg)
	if err != nil {
		t.Fatalf("ConfigFromSDKConfig() error = %v", err)
	}
	if !configured {
		t.Fatal("expected environment-only AI configuration to be enabled")
	}
	if aiConfig.Provider != "openai" || aiConfig.Model != "env-model" || aiConfig.APIKey != "env-credential" ||
		aiConfig.BaseURL != "https://gateway.example/v1" || aiConfig.Timeout != 75*time.Second || aiConfig.MaxTokens != 2048 {
		t.Fatalf("unexpected environment-only config: %s", aiConfig)
	}
}

// Environment values reach a hand-built Config only through Viper or
// config.NewConfig, so a stray PVTR_AI_* export cannot enable or redirect AI in
// a process that never configured either.
func TestConfigFromSDKConfig_ProcessEnvironmentDoesNotReachHandBuiltVars(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("PVTR_AI_MODEL", "env-model")
	t.Setenv("PVTR_AI_MAX_TOKENS", "4096")
	t.Setenv("PVTR_AI_API_KEY", "sk-ambient-credential")

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured AI")
	}
	if aiConfig.BaseURL != "" {
		t.Fatalf("BaseURL = %q, want the process environment to be invisible here", aiConfig.BaseURL)
	}
	if aiConfig.Model != "gpt-4o-mini" || aiConfig.MaxTokens != defaultMaxTokens || aiConfig.APIKey != "test-key" {
		t.Fatalf("hand-built Vars did not win over the environment: %s", aiConfig)
	}
}

// Without Viper's env binding an ambient credential must not stand in for a
// missing one, so a hand-built Config cannot be silently completed from the
// shell of whatever process happens to be running.
func TestConfigFromSDKConfig_AmbientCredentialDoesNotCompleteHandBuiltVars(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_API_KEY", "sk-ambient-credential")

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured AI")
	}
	if aiConfig.APIKey != "" {
		t.Fatalf("APIKey = %q, want the ambient credential to be ignored", aiConfig.APIKey)
	}
}

func TestConfigFromSDKConfig_RejectsNonPositiveEnvironmentMaxTokens(t *testing.T) {
	for _, maxTokens := range []string{"0", "-1"} {
		t.Run(maxTokens, func(t *testing.T) {
			t.Setenv("PVTR_AI_PROVIDER", "openai")
			t.Setenv("PVTR_AI_MODEL", "env-model")
			t.Setenv("PVTR_AI_BASE_URL", "https://gateway.example/v1")
			t.Setenv("PVTR_AI_MAX_TOKENS", maxTokens)
			cfg := configFromFileForTarget(t, `
services:
  repo-one:
    policy: {catalogs: [catalog], applicability: [all]}
`, "repo-one")

			_, configured, err := ConfigFromSDKConfig(cfg)
			if configured {
				t.Fatal("expected an invalid configuration to report AI as not enabled")
			}
			if err == nil || !strings.Contains(err.Error(), "ai_max_tokens must be positive") {
				t.Fatalf("error = %v, want positive ai_max_tokens error", err)
			}
		})
	}
}

func TestConfigFromSDKConfig_EnvironmentOverridesTargetAndTopLevel(t *testing.T) {
	t.Setenv("PVTR_AI_PROVIDER", "anthropic")
	t.Setenv("PVTR_AI_MODEL", "env-model")
	t.Setenv("PVTR_AI_BASE_URL", "https://env.example/v1")
	t.Setenv("PVTR_AI_TIMEOUT", "90s")
	t.Setenv("PVTR_AI_MAX_TOKENS", "4096")
	cfg := configFromFileForTarget(t, `
ai_provider: openai
ai_model: top-model
ai_base_url: https://top.example/v1
ai_timeout: 30s
ai_max_tokens: 512
services:
  repo-one:
    vars:
      ai_provider: openai
      ai_model: target-model
      ai_base_url: https://target.example/v1
      ai_timeout: 60s
      ai_max_tokens: 1024
    policy: {catalogs: [catalog], applicability: [all]}
`, "repo-one")

	aiConfig, configured, err := ConfigFromSDKConfig(cfg)
	if err != nil {
		t.Fatalf("ConfigFromSDKConfig() error = %v", err)
	}
	if !configured {
		t.Fatal("expected configured AI")
	}
	if aiConfig.Provider != "anthropic" || aiConfig.Model != "env-model" || aiConfig.BaseURL != "https://env.example/v1" ||
		aiConfig.Timeout != 90*time.Second || aiConfig.MaxTokens != 4096 {
		t.Fatalf("environment did not override target and top-level settings: %s", aiConfig)
	}
}

func TestConfigFromSDKConfig_TargetOverridesInheritedSettings(t *testing.T) {
	cfg := configFromFileForTarget(t, `
ai_provider: openai
ai_model: top-model
ai_base_url: https://top.example/v1
ai_timeout: 30s
ai_max_tokens: 512
services:
  repo-one:
    vars:
      ai_model: target-model
      ai_timeout: 60s
      ai_max_tokens: 2048
    policy: {catalogs: [catalog], applicability: [all]}
`, "repo-one")

	aiConfig, configured, err := ConfigFromSDKConfig(cfg)
	if err != nil {
		t.Fatalf("ConfigFromSDKConfig() error = %v", err)
	}
	if !configured {
		t.Fatal("expected configured AI")
	}
	if aiConfig.Provider != "openai" || aiConfig.Model != "target-model" || aiConfig.BaseURL != "https://top.example/v1" ||
		aiConfig.Timeout != 60*time.Second || aiConfig.MaxTokens != 2048 {
		t.Fatalf("unexpected inherited and overridden config: %s", aiConfig)
	}
}

func TestConfigFromSDKConfig_MixedTargets(t *testing.T) {
	const configText = `
services:
  ai-target:
    vars:
      ai_provider: openai
      ai_model: target-model
      ai_base_url: https://gateway.example/v1
    policy: {catalogs: [catalog], applicability: [all]}
  no-ai-target:
    vars: {owner: privateerproj}
    policy: {catalogs: [catalog], applicability: [all]}
`

	for _, tt := range []struct {
		target         string
		wantConfigured bool
	}{
		{target: "ai-target", wantConfigured: true},
		{target: "no-ai-target", wantConfigured: false},
	} {
		t.Run(tt.target, func(t *testing.T) {
			cfg := configFromFileForTarget(t, configText, tt.target)
			_, configured, err := ConfigFromSDKConfig(cfg)
			if err != nil {
				t.Fatalf("ConfigFromSDKConfig() error = %v", err)
			}
			if configured != tt.wantConfigured {
				t.Fatalf("configured = %t, want %t", configured, tt.wantConfigured)
			}
		})
	}
}

func TestConfigFromSDKConfig_UnsupportedAPIKeyEnvDoesNotMaskFileValue(t *testing.T) {
	t.Setenv("PVTR_AI_API_KEY_ENV", "IGNORED_VARIABLE_NAME")
	t.Setenv("PRIVATEER_TEST_FILE_AI_KEY", "file-named-credential")
	cfg := configFromFileForTarget(t, `
ai_provider: openai
ai_model: top-model
ai_api_key_env: PRIVATEER_TEST_FILE_AI_KEY
services:
  repo-one:
    policy: {catalogs: [catalog], applicability: [all]}
`, "repo-one")

	aiConfig, configured, err := ConfigFromSDKConfig(cfg)
	if err != nil {
		t.Fatalf("ConfigFromSDKConfig() error = %v", err)
	}
	if !configured || aiConfig.APIKey != "file-named-credential" {
		t.Fatalf("expected file ai_api_key_env to resolve, got configured=%t config=%s", configured, aiConfig)
	}
}

func TestConfigFromSDKConfig_APIKeyEnv(t *testing.T) {
	const envName = "PRIVATEER_TEST_TARGET_AI_KEY"

	vars := map[string]interface{}{
		"ai_provider":    "openai",
		"ai_model":       "gpt-4o-mini",
		"ai_api_key_env": envName,
	}

	t.Run("resolves the named variable", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		t.Setenv(envName, "secret-from-env")

		aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: vars})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !configured {
			t.Fatal("expected configured AI")
		}
		if aiConfig.APIKey != "secret-from-env" {
			t.Fatalf("APIKey = %q, want credential from %s", aiConfig.APIKey, envName)
		}
	})

	t.Run("rejects an unset variable", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		t.Setenv(envName, "")

		_, _, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: vars})
		if err == nil || !strings.Contains(err.Error(), "unset or empty") {
			t.Fatalf("expected unset ai_api_key_env error, got %v", err)
		}
	})
}

func TestConfigFromSDKConfig_HandBuiltTargetCredentialsPrecedeProcessEnvironment(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]interface{}
		want string
	}{
		{
			name: "direct credential",
			vars: map[string]interface{}{
				"ai_api_key": "target-direct",
			},
			want: "target-direct",
		},
		{
			name: "named credential",
			vars: map[string]interface{}{
				"ai_api_key_env": "PRIVATEER_TEST_HAND_BUILT_AI_KEY",
			},
			want: "target-named",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("PVTR_AI_API_KEY", "process-key")
			t.Setenv("PRIVATEER_TEST_HAND_BUILT_AI_KEY", "target-named")
			tt.vars["ai_provider"] = "openai"
			tt.vars["ai_model"] = "gpt-4o-mini"

			aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{ServiceName: "repo-one", Vars: tt.vars})
			if err != nil {
				t.Fatalf("ConfigFromSDKConfig() error = %v", err)
			}
			if !configured {
				t.Fatal("expected configured AI")
			}
			if aiConfig.APIKey != tt.want {
				t.Fatalf("APIKey = %q, want target credential %q", aiConfig.APIKey, tt.want)
			}
		})
	}
}

func TestConfigFromSDKConfig_APIKeyPrecedence(t *testing.T) {
	const baseConfig = `
ai_provider: openai
ai_model: gpt-4o-mini
%s
services:
  repo-one:
    policy:
      catalogs: [catalog]
      applicability: [all]
%s
`

	tests := []struct {
		name       string
		topLevel   string
		target     string
		processKey string
		namedKey   string
		want       string
	}{
		{
			name:       "target direct beats target environment and process environment",
			target:     "    vars:\n      ai_api_key: target-direct\n      ai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			processKey: "process-key",
			namedKey:   "named-key",
			want:       "target-direct",
		},
		{
			name:       "target environment beats process environment",
			target:     "    vars:\n      ai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			processKey: "process-key",
			namedKey:   "named-key",
			want:       "named-key",
		},
		{
			name:       "process environment beats top-level sources",
			topLevel:   "ai_api_key: top-level-direct\nai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			processKey: "process-key",
			namedKey:   "named-key",
			want:       "process-key",
		},
		{
			name:     "top-level direct beats top-level environment",
			topLevel: "ai_api_key: top-level-direct\nai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			namedKey: "named-key",
			want:     "top-level-direct",
		},
		{
			name:     "top-level environment is final configured source",
			topLevel: "ai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			namedKey: "named-key",
			want:     "named-key",
		},
		{
			name:       "empty target direct falls through",
			target:     "    vars:\n      ai_api_key: \"\"\n      ai_api_key_env: PRIVATEER_TEST_NAMED_AI_KEY",
			processKey: "process-key",
			namedKey:   "named-key",
			want:       "named-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.SetEnvPrefix("PVTR")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			viper.AutomaticEnv()
			t.Setenv("PVTR_AI_API_KEY", tt.processKey)
			t.Setenv("PRIVATEER_TEST_NAMED_AI_KEY", tt.namedKey)
			viper.SetConfigType("yaml")
			if err := viper.ReadConfig(bytes.NewBufferString(fmt.Sprintf(baseConfig, tt.topLevel, tt.target))); err != nil {
				t.Fatalf("ReadConfig() error = %v", err)
			}
			viper.Set("service", "repo-one")

			aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.NewConfig(nil))
			if err != nil {
				t.Fatalf("ConfigFromSDKConfig() error = %v", err)
			}
			if !configured {
				t.Fatal("expected configured AI")
			}
			if aiConfig.APIKey != tt.want {
				t.Fatalf("APIKey = %q, want %q", aiConfig.APIKey, tt.want)
			}
		})
	}
}

func TestConfigFromSDKConfig_TargetAPIKeyEnvDoesNotFallThrough(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("PVTR")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	t.Setenv("PVTR_AI_API_KEY", "process-fallback")
	t.Setenv("PRIVATEER_TEST_MISSING_AI_KEY", "")
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBufferString(`
ai_provider: openai
ai_model: gpt-4o-mini
ai_api_key: top-level-fallback
services:
  repo-one:
    vars:
      ai_api_key_env: PRIVATEER_TEST_MISSING_AI_KEY
    policy:
      catalogs: [catalog]
      applicability: [all]
`)); err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}
	viper.Set("service", "repo-one")

	_, configured, err := ConfigFromSDKConfig(sdkconfig.NewConfig(nil))
	if configured {
		t.Fatal("expected an invalid configuration to report AI as not enabled")
	}
	if err == nil || !strings.Contains(err.Error(), "unset or empty") {
		t.Fatalf("error = %v, want unset ai_api_key_env error", err)
	}
}

func TestConfigFromSDKConfig_DoesNotResolveAPIKeyEnvThroughViper(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ai_provider", "openai")
	viper.Set("ai_model", "gpt-4o-mini")
	viper.Set("ai_api_key_env", "SHOULD_NOT_BE_READ")

	aiConfig, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !configured {
		t.Fatal("expected configured AI")
	}
	if aiConfig.APIKey != "" {
		t.Fatalf("expected ai_api_key_env viper value to be ignored, got %q", aiConfig.APIKey)
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
	if configured {
		t.Fatal("expected an invalid configuration to report AI as not enabled")
	}
	if err == nil {
		t.Fatal("expected invalid timeout error, got nil")
	}
}

func TestConfigFromSDKConfig_RejectsNonPositiveLimits(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]interface{}
		wantErrText string
	}{
		{
			name: "zero timeout",
			vars: map[string]interface{}{
				"ai_provider": "openai",
				"ai_timeout":  "0s",
			},
			wantErrText: "ai_timeout must be positive",
		},
		{
			name: "negative max tokens",
			vars: map[string]interface{}{
				"ai_provider":   "openai",
				"ai_max_tokens": -1,
			},
			wantErrText: "ai_max_tokens must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: tt.vars})
			if configured {
				t.Fatal("expected an invalid configuration to report AI as not enabled")
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErrText)
			}
		})
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
		{
			name: "bool field with string value",
			vars: map[string]interface{}{
				"ai_provider": "openai",
				"ai_skip":     "true",
			},
			wantErrText: "ai_skip must be a bool, got string",
		},
		{
			name: "api key with int value",
			vars: map[string]interface{}{
				"ai_provider": "openai",
				"ai_model":    "gpt-4o-mini",
				"ai_api_key":  123,
			},
			wantErrText: "ai_api_key must be a string, got int",
		},
		{
			name: "api key env with int value",
			vars: map[string]interface{}{
				"ai_provider":    "openai",
				"ai_model":       "gpt-4o-mini",
				"ai_api_key_env": 123,
			},
			wantErrText: "ai_api_key_env must be a string, got int",
		},
		{
			name: "api key env with empty value",
			vars: map[string]interface{}{
				"ai_provider":    "openai",
				"ai_model":       "gpt-4o-mini",
				"ai_api_key_env": "",
			},
			wantErrText: "ai_api_key_env must name a non-empty environment variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			_, configured, err := ConfigFromSDKConfig(sdkconfig.Config{Vars: tt.vars})
			if configured {
				t.Fatal("expected an invalid configuration to report AI as not enabled")
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
