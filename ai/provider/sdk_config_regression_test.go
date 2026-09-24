package provider

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

// A Config carrying an error must never reach a provider: NewConfig reports
// unresolvable AI settings that way, and the rest of the Vars look usable.
func TestConfigFromSDKConfig_RefusesConfigCarryingAnError(t *testing.T) {
	cfg := sdkconfig.Config{
		Error: errors.New("load configuration with config.ReadConfig"),
		Vars: map[string]interface{}{
			"ai_provider": "openai", "ai_model": "model", "ai_api_key": "key",
		},
	}
	result, enabled, err := ConfigFromSDKConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "load configuration with config.ReadConfig") {
		t.Fatalf("err = %v, want the configuration error surfaced", err)
	}
	if enabled || !reflect.DeepEqual(result, Config{}) {
		t.Fatalf("enabled=%v, config=%s; want an errored config refused outright", enabled, result)
	}
}

func TestConfigFromSDKConfig_DormantSkipErrorsAreIgnored(t *testing.T) {
	for _, value := range []interface{}{"true", 123, nil} {
		t.Run(fmt.Sprintf("%T", value), func(t *testing.T) {
			cfg := sdkconfig.Config{Vars: map[string]interface{}{
				"ai_skip": value, "ai_api_key_env": 42, "ai_timeout": false,
			}}
			result, enabled, err := ConfigFromSDKConfig(cfg)
			if err != nil || enabled || !reflect.DeepEqual(result, Config{}) {
				t.Fatalf("dormant AI returned enabled=%v, err=%v", enabled, err)
			}
		})
	}
}

func TestConfigFromSDKConfig_HandBuiltVarsIgnoreAllGlobalState(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("PVTR")
	viper.AutomaticEnv()
	for key, value := range map[string]string{
		"ai_provider": "openai", "ai_model": "ambient-model",
		"ai_api_key": "ambient-key", "ai_base_url": "http://localhost:11434/v1",
		"ai_skip": "true", "ai_max_tokens": "4096",
	} {
		t.Setenv("PVTR_"+strings.ToUpper(key), value)
		viper.Set(key, value)
	}
	viper.Set("targets.repo.vars.ai_api_key", "unrelated-target-key")

	_, enabled, err := ConfigFromSDKConfig(sdkconfig.Config{ServiceName: "repo"})
	if err != nil || enabled {
		t.Fatalf("global state enabled an empty config: enabled=%v, err=%v", enabled, err)
	}
	cfg := sdkconfig.Config{ServiceName: "repo", Vars: map[string]interface{}{
		"ai_provider": "openai", "ai_model": "explicit-model", "ai_skip": false,
	}}
	result, enabled, err := ConfigFromSDKConfig(cfg)
	if err != nil || !enabled || result.APIKey != "" || result.BaseURL != "" ||
		result.Model != "explicit-model" || result.MaxTokens != defaultMaxTokens {
		t.Fatalf("hand-built settings were changed by global state: %s, enabled=%v, err=%v", result, enabled, err)
	}
}

func TestConfigFromSDKConfig_CredentialLadderAcrossTargets(t *testing.T) {
	tests := []struct {
		name, top, local, process, want, wantErr string
	}{
		{"target named before all literals", "ai_api_key_env: PVTR_AI_TEST_TOP_KEY\nai_api_key: top-literal",
			"ai_api_key_env: PVTR_AI_TEST_TARGET_KEY\n      ai_api_key: target-literal", "process", "target-named", ""},
		{"process before target literal", "ai_api_key: top-literal", "ai_api_key: target-literal", "process", "process", ""},
		{"process before top named", "ai_api_key_env: PVTR_AI_TEST_TOP_KEY\nai_api_key: top-literal",
			"ai_api_key: target-literal", "process", "process", ""},
		{"top named before top literal", "ai_api_key_env: PVTR_AI_TEST_TOP_KEY\nai_api_key: top-literal", "", "", "top-named", ""},
		{"empty target literal falls through", "ai_api_key: top-literal", "ai_api_key: ''", "", "top-literal", ""},
		{"top named before target literal", "ai_api_key_env: PVTR_AI_TEST_TOP_KEY", "ai_api_key: target-literal", "", "top-named", ""},
		{"target literal before top literal", "ai_api_key: top-literal", "ai_api_key: target-literal", "", "target-literal", ""},
		{"global named before flat named", "ai_api_key_env: PVTR_AI_TEST_TARGET_KEY\nvars: {ai_api_key_env: PVTR_AI_TEST_TOP_KEY}",
			"ai_api_key: target-literal", "", "top-named", ""},
		{"process before global named", "vars: {ai_api_key_env: PVTR_AI_TEST_TOP_KEY}", "", "process", "process", ""},
		{"global literal before flat literal", "ai_api_key: top-literal\nvars: {ai_api_key: global-literal}",
			"", "", "global-literal", ""},
		{"target literal before global literal", "vars: {ai_api_key: global-literal}", "ai_api_key: target-literal", "", "target-literal", ""},
		{"empty global literal falls through to flat spelling", "ai_api_key: top-literal\nvars: {ai_api_key: \"\"}",
			"", "", "top-literal", ""},
		{"missing target named does not fall through", "ai_api_key: top-literal",
			"ai_api_key_env: PVTR_AI_TEST_MISSING_KEY", "process", "", "unset or empty"},
		{"empty target variable name does not fall through", "ai_api_key: top-literal",
			"ai_api_key_env: \"\"", "process", "", "non-empty environment variable"},
		{"missing top named does not fall through", "ai_api_key_env: PVTR_AI_TEST_MISSING_KEY",
			"ai_api_key: target-literal", "", "", "unset or empty"},
		{"null top named does not fall through", "ai_api_key_env: null",
			"ai_api_key: target-literal", "", "", "must be a string"},
		{"unselected malformed top source ignored", "ai_api_key_env: 123",
			"", "process", "process", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PVTR_AI_API_KEY", tt.process)
			t.Setenv("PVTR_AI_TEST_TARGET_KEY", "target-named")
			t.Setenv("PVTR_AI_TEST_TOP_KEY", "top-named")
			t.Setenv("PVTR_AI_TEST_MISSING_KEY", "")
			cfg := configFromFileForTarget(t, fmt.Sprintf(`
ai_provider: openai
ai_model: model
%s
targets:
  repo:
    policy: {catalogs: [catalog], applicability: [all]}
    vars:
      %s
`, tt.top, tt.local), "repo")
			result, enabled, err := ConfigFromSDKConfig(cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || enabled {
					t.Fatalf("enabled=%v, err=%v; want %q", enabled, err, tt.wantErr)
				}
				return
			}
			if err != nil || !enabled || result.APIKey != tt.want {
				t.Fatalf("incorrect credential selection: enabled=%v, err=%v", enabled, err)
			}
		})
	}
}

func TestConfigFromSDKConfig_NoTargetCredentialUsesEnvironment(t *testing.T) {
	for _, target := range []string{"", "missing-target"} {
		t.Run("target="+target, func(t *testing.T) {
			t.Setenv("PVTR_AI_API_KEY", "process-key")
			cfg := configFromFileForTarget(t, "ai_provider: openai\nai_model: model\nai_api_key: file-key\npolicy: {catalogs: [catalog], applicability: [all]}\n", target)
			result, enabled, err := ConfigFromSDKConfig(cfg)
			if err != nil || !enabled || result.APIKey != "process-key" {
				t.Fatalf("no-target credential precedence failed: enabled=%v, err=%v", enabled, err)
			}
		})
	}
}

func TestConfigFromSDKConfig_MaterializedConfigIsStable(t *testing.T) {
	t.Setenv("PVTR_AI_MODEL", "original-model")
	t.Setenv("PVTR_AI_API_KEY", "original-key")
	cfg := configFromFileForTarget(t, "ai_provider: openai\n", "")
	viper.Reset()
	viper.Set("ai_skip", true)
	t.Setenv("PVTR_AI_MODEL", "changed-model")
	t.Setenv("PVTR_AI_API_KEY", "changed-key")
	result, enabled, err := ConfigFromSDKConfig(cfg)
	if err != nil || !enabled || result.Model != "original-model" || result.APIKey != "original-key" {
		t.Fatalf("materialized config changed: enabled=%v, err=%v", enabled, err)
	}
}

func TestConfigFromSDKConfig_WhitespaceEnvironmentDoesNotOverrideDefaults(t *testing.T) {
	for _, key := range []string{"ai_skip", "ai_max_tokens", "ai_timeout", "ai_api_key_env"} {
		t.Setenv("PVTR_"+strings.ToUpper(key), "   ")
	}
	cfg := configFromFileForTarget(t, "ai_provider: openai\nai_model: model\nai_base_url: http://localhost:11434/v1\n", "")
	result, enabled, err := ConfigFromSDKConfig(cfg)
	if err != nil || !enabled || result.Timeout != defaultTimeout || result.MaxTokens != defaultMaxTokens {
		t.Fatalf("whitespace environment overrode defaults: enabled=%v, err=%v", enabled, err)
	}
}

func TestConfigFromSDKConfig_NullFlatSkipIsInvalidWhenEnabled(t *testing.T) {
	cfg := configFromFileForTarget(t, "ai_provider: openai\nai_skip: null\n", "")
	_, enabled, err := ConfigFromSDKConfig(cfg)
	if enabled || err == nil || !strings.Contains(err.Error(), "ai_skip must be a bool") {
		t.Fatalf("null skip was ignored: enabled=%v, err=%v", enabled, err)
	}
}
