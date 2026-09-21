package config

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ignoredSettingsConfig misspells a key at every scope the resolver reads, plus
// one correctly spelled key, so a report that drops a scope or flags a good key
// is visible in one assertion.
const ignoredSettingsConfig = `
ai_provider: openai
ai_model: model
ai_modle: root-typo
vars:
  ai_skipp: shared-typo
targets:
  repo:
    vars:
      ai_provder: target-typo
    policy: {catalogs: [catalog], applicability: [all]}
  other:
    vars:
      ai_unselected_typo: other-target
    policy: {catalogs: [catalog], applicability: [all]}
`

func loadConfig(t *testing.T, document string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := ReadConfig(strings.NewReader(document)); err != nil {
		t.Fatal(err)
	}
}

func aiSourcesForTarget(name string) aiSettingSources {
	return aiSettingSources{
		target: viper.GetStringMap("targets." + name + ".vars"),
		shared: viper.GetStringMap("vars"),
		file:   rawFileSettings(),
	}
}

func TestIgnoredAISettings_ReportsTyposAtEveryReadScope(t *testing.T) {
	loadConfig(t, ignoredSettingsConfig)
	t.Setenv("PVTR_AI_TIMEOUTT", "30s")
	t.Setenv("PVTR_AI_MODEL", "env-model")
	t.Setenv("PVTR_TARGET", "repo")

	got := ignoredAISettings(aiSourcesForTarget("repo"))

	want := []string{"PVTR_AI_TIMEOUTT", "ai_modle", "ai_provder", "ai_skipp"}
	if !slices.Equal(got, want) {
		t.Fatalf("ignoredAISettings() = %v, want %v", got, want)
	}
}

func TestIgnoredAISettings_ReportsEnvironmentSpellingOfConfigOnlyKeys(t *testing.T) {
	loadConfig(t, "ai_provider: openai\nai_model: model\n")
	t.Setenv("PVTR_AI_API_KEY_ENV", "SOME_KEY")

	got := ignoredAISettings(aiSourcesForTarget("repo"))

	// The config spelling is supported; only the environment one is ignored.
	if !slices.Equal(got, []string{"PVTR_AI_API_KEY_ENV"}) {
		t.Fatalf("ignoredAISettings() = %v, want [PVTR_AI_API_KEY_ENV]", got)
	}
}

func TestIgnoredAISettings_AcceptsEveryRecognizedKey(t *testing.T) {
	loadConfig(t, `
ai_provider: openai
ai_model: model
ai_base_url: https://gateway.example/v1
ai_timeout: 10s
ai_max_tokens: 64
ai_skip: false
ai_api_key: literal
ai_api_key_env: SOME_KEY
`)

	if got := ignoredAISettings(aiSourcesForTarget("repo")); len(got) != 0 {
		t.Fatalf("ignoredAISettings() = %v, want none; a recognized key was reported as a typo", got)
	}
}

func TestAIEnabledByEnvironmentOnly(t *testing.T) {
	tests := []struct {
		name, document, environment string
		want                        bool
	}{
		{"environment alone enables", "ai_model: model\n", "openai", true},
		{"file provider is not environment-only", "ai_provider: anthropic\nai_model: model\n", "openai", false},
		{"shared vars provider is not environment-only", "vars: {ai_provider: anthropic}\n", "openai", false},
		{"no environment variable", "ai_provider: openai\nai_model: model\n", "", false},
		{"blank environment variable", "ai_model: model\n", "   ", false},
		{"opted out", "ai_skip: true\nai_model: model\n", "openai", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadConfig(t, tt.document)
			t.Setenv("PVTR_AI_PROVIDER", tt.environment)

			sources := aiSourcesForTarget("repo")
			resolved := map[string]interface{}{}
			if err := applyAIPrecedenceRules(resolved, sources); err != nil {
				t.Fatalf("applyAIPrecedenceRules() error = %v", err)
			}
			if got := aiEnabledByEnvironmentOnly(resolved, sources); got != tt.want {
				t.Fatalf("aiEnabledByEnvironmentOnly() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAIEnablementHint(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv("PVTR_AI_PROVIDER", "openai")
		hint := AIEnablementHint()
		if !strings.Contains(hint, "PVTR_AI_PROVIDER") || !strings.Contains(hint, "ai_skip") {
			t.Fatalf("AIEnablementHint() = %q, want it to name the variable and the opt-out", hint)
		}
	})
	t.Run("unset", func(t *testing.T) {
		t.Setenv("PVTR_AI_PROVIDER", "")
		if hint := AIEnablementHint(); hint != "" {
			t.Fatalf("AIEnablementHint() = %q, want empty", hint)
		}
	})
}

// A blank literal is an unfilled placeholder at every scope, not a deliberate
// instruction to send no credential, so it must not mask a configured one.
func TestApplyAICredential_BlankSharedLiteralFallsThroughToRootLiteral(t *testing.T) {
	loadConfig(t, "ai_provider: openai\nai_model: model\nai_api_key: root-literal\nvars: {ai_api_key: \"\"}\n")
	t.Setenv("PVTR_AI_API_KEY", "")

	resolved := map[string]interface{}{}
	if err := applyAIPrecedenceRules(resolved, aiSourcesForTarget("repo")); err != nil {
		t.Fatalf("applyAIPrecedenceRules() error = %v", err)
	}
	if got := resolved["ai_api_key"]; got != "root-literal" {
		t.Fatalf("ai_api_key = %v, want %q; a blank shared placeholder masked the configured literal", got, "root-literal")
	}
}
