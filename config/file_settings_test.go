package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestReadConfig_PreservesUnshadowedAISettings(t *testing.T) {
	for _, format := range []string{"yaml", "json", "toml"} {
		t.Run(format, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.SetConfigType(format)
			viper.SetEnvPrefix("PVTR")
			viper.AutomaticEnv()
			t.Setenv("PVTR_AI_SKIP", "false")
			t.Setenv("PVTR_AI_API_KEY_ENV", "IGNORED_ENV_NAME")
			content := map[string]string{
				"yaml": "AI_SKIP: true\nAI_API_KEY_ENV: FILE_KEY_NAME\n",
				"json": `{"AI_SKIP":true,"AI_API_KEY_ENV":"FILE_KEY_NAME"}`,
				"toml": "AI_SKIP = true\nAI_API_KEY_ENV = 'FILE_KEY_NAME'\n",
			}[format]
			if err := ReadConfig(strings.NewReader(content)); err != nil {
				t.Fatal(err)
			}
			cfg := NewConfig(nil)
			if cfg.Error != nil || !cfg.GetBool("ai_skip") ||
				cfg.GetString("ai_api_key_env") != "FILE_KEY_NAME" {
				t.Fatalf("raw settings were shadowed: %v", cfg.Error)
			}
		})
	}
}

func TestReadConfig_ReplacementUpdatesRawSettings(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("PVTR")
	viper.AutomaticEnv()
	t.Setenv("PVTR_AI_SKIP", "false")
	t.Setenv("PVTR_AI_API_KEY_ENV", "IGNORED")
	for _, skip := range []string{"true", "false", "true"} {
		if err := ReadConfig(strings.NewReader("ai_skip: " + skip + "\nai_api_key_env: FILE_KEY\n")); err != nil {
			t.Fatal(err)
		}
		cfg := NewConfig(nil)
		if cfg.Error != nil || cfg.GetBool("ai_skip") != (skip == "true") {
			t.Fatalf("replacement read used stale settings: skip=%s, err=%v", skip, cfg.Error)
		}
	}
	// The installed decoder also observes subsequent direct replacement reads.
	if err := viper.ReadConfig(strings.NewReader("ai_skip: false\n")); err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig(nil)
	if cfg.Error != nil || cfg.GetBool("ai_skip") || cfg.GetString("ai_api_key_env") != "" {
		t.Fatalf("direct replacement retained old settings: %v", cfg.Error)
	}
}

func TestReadConfig_FailedReplacementRetainsLastValidSettings(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := ReadConfig(strings.NewReader("ai_skip: true\n")); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(strings.NewReader("invalid: [")); err == nil {
		t.Fatal("expected parse error")
	}
	raw := rawFileSettings()
	if raw == nil || !raw.GetBool("ai_skip") || !viper.GetBool("ai_skip") {
		t.Fatal("failed load desynchronized captured settings from Viper's last valid state")
	}
}

func TestReadInConfig_DeletedFileRetainsOptOut(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("PVTR")
	viper.AutomaticEnv()
	t.Setenv("PVTR_AI_SKIP", "false")
	filename := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(filename, []byte("ai_skip: true\nai_provider: openai\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	viper.SetConfigFile(filename)
	if err := ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		cfg := NewConfig(nil)
		if cfg.Error != nil || !cfg.GetBool("ai_skip") {
			t.Fatalf("configuration was reread instead of using captured opt-out: %v", cfg.Error)
		}
	}
}

func TestNewConfig_UncapturedMaskedFileSettingsFailClosed(t *testing.T) {
	for _, key := range []string{"ai_skip", "ai_api_key_env"} {
		t.Run(key, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.SetConfigType("yaml")
			viper.SetEnvPrefix("PVTR")
			viper.AutomaticEnv()
			t.Setenv("PVTR_"+strings.ToUpper(key), "false")
			if err := viper.ReadConfig(strings.NewReader("ai_provider: openai\n" + key + ": true\n")); err != nil {
				t.Fatal(err)
			}
			cfg := NewConfig(nil)
			if cfg.Error == nil || !strings.Contains(cfg.Error.Error(), "config.ReadConfig") {
				t.Fatal("uncaptured masked file setting did not fail closed")
			}
		})
	}
}

func TestReadConfig_CaptureDoesNotShareMutableMaps(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := ReadConfig(strings.NewReader("vars: {ai_skip: true}\n")); err != nil {
		t.Fatal(err)
	}
	viper.GetStringMap("vars")["ai_skip"] = false
	raw := rawFileSettings()
	if raw == nil || !raw.GetBool("vars.ai_skip") {
		t.Fatal("captured file settings share mutable maps with Viper")
	}
}

func TestNewConfig_ProgrammaticOverridesAfterFileLoad(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := ReadConfig(strings.NewReader(`
ai_provider: openai
ai_model: file-model
ai_base_url: https://file.example/v1
ai_api_key_env: FILE_KEY_NAME
ai_skip: false
`)); err != nil {
		t.Fatal(err)
	}
	viper.Set("ai_provider", "anthropic")
	viper.Set("ai_model", "programmatic-model")
	viper.Set("ai_base_url", "https://programmatic.example/v1")
	viper.Set("ai_api_key_env", "PROGRAMMATIC_KEY_NAME")
	viper.Set("ai_skip", true)
	cfg := NewConfig(nil)
	if cfg.Error != nil || !cfg.GetBool("ai_skip") || cfg.GetString("ai_model") != "programmatic-model" ||
		cfg.GetString("ai_provider") != "anthropic" || cfg.GetString("ai_base_url") != "https://programmatic.example/v1" ||
		cfg.GetString("ai_api_key_env") != "PROGRAMMATIC_KEY_NAME" {
		t.Fatalf("programmatic overrides were lost: %v", cfg.Error)
	}
}

func TestNewConfig_FileSkipTrueWinsOverProgrammaticFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := ReadConfig(strings.NewReader("ai_skip: true\n")); err != nil {
		t.Fatal(err)
	}
	viper.Set("ai_skip", false)
	cfg := NewConfig(nil)
	if cfg.Error != nil || !cfg.GetBool("ai_skip") {
		t.Fatalf("programmatic false hid file opt-out: %v", cfg.Error)
	}
}

func TestNewConfig_UncapturedProvenanceOnlyRequiredWhenUsed(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		bindEnv    bool
	}{
		{"dormant", "ai_skip: false\n", true},
		{"dormant whitespace provider", "ai_provider: '  '\nai_skip: false\n", true},
		{"independent opt-out", "ai_provider: openai\nai_skip: false\nvars: {ai_skip: true}\n", true},
		{"unbound environment", "ai_provider: openai\nai_skip: false\n", false},
		{"unused credential source", "ai_provider: openai\nai_api_key_env: FILE_KEY\nvars: {ai_api_key_env: SELECTED_KEY}\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.SetConfigType("yaml")
			t.Setenv("PVTR_AI_SKIP", "false")
			t.Setenv("PVTR_AI_API_KEY_ENV", "ENV_KEY_NAME")
			if tt.bindEnv {
				viper.SetEnvPrefix("PVTR")
				viper.AutomaticEnv()
			}
			if err := viper.ReadConfig(strings.NewReader(tt.text)); err != nil {
				t.Fatal(err)
			}
			if cfg := NewConfig(nil); cfg.Error != nil {
				t.Fatalf("irrelevant provenance rejected configuration: %v", cfg.Error)
			}
		})
	}
}

// Decoders may hand back map[interface{}]interface{} rather than string-keyed
// maps; those have to be deep-copied too, or a later write into the decoded
// document would silently rewrite the captured file settings.
func TestCloneFileValue_DeepCopiesNonStringKeyedMaps(t *testing.T) {
	source := map[string]interface{}{
		"targets": map[interface{}]interface{}{
			"repo": map[string]interface{}{"ai_skip": true},
			1:      []interface{}{map[interface{}]interface{}{"ai_model": "original"}},
		},
	}

	clone, isMap := cloneFileValue(source).(map[string]interface{})
	if !isMap {
		t.Fatalf("clone type = %T, want map[string]interface{}", cloneFileValue(source))
	}

	decoded := source["targets"].(map[interface{}]interface{})
	decoded["repo"].(map[string]interface{})["ai_skip"] = false
	decoded[1].([]interface{})[0].(map[interface{}]interface{})["ai_model"] = "mutated"

	captured := clone["targets"].(map[interface{}]interface{})
	if got := captured["repo"].(map[string]interface{})["ai_skip"]; got != true {
		t.Errorf("ai_skip = %v, want true; the capture shares the decoded map", got)
	}
	if got := captured[1].([]interface{})[0].(map[interface{}]interface{})["ai_model"]; got != "original" {
		t.Errorf("ai_model = %v, want %q; the capture shares a nested slice element", got, "original")
	}
}
