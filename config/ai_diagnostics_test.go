package config

import (
	"bytes"
	"io"
	"os"
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

// captureStderr collects what NewConfig actually emits. The diagnostics are
// only useful if they survive the default log level, so these tests assert on
// emitted bytes rather than on the branch being taken.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	done := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		done <- buffer.String()
	}()
	run()
	os.Stderr = original
	_ = writer.Close()
	output := <-done
	_ = reader.Close()
	return output
}

func TestNewConfig_DiagnosticsSurviveTheDefaultLogLevel(t *testing.T) {
	tests := []struct {
		name, document, loglevel string
		environment              map[string]string
		want, notWant            []string
	}{
		{
			name: "default log level still shows every finding",
			// No ai_provider in the file: the environment is what enables AI.
			document:    "ai_model: model\nai_api_key: literal\nai_provder: typo\n",
			environment: map[string]string{"PVTR_AI_PROVIDER": "openai"},
			want:        []string{"remove plaintext credentials", "ai_provder", "PVTR_AI_PROVIDER"},
		},
		{
			name:        "explicit off is honored",
			document:    "ai_model: model\nai_api_key: literal\nai_provder: typo\n",
			loglevel:    "off",
			environment: map[string]string{"PVTR_AI_PROVIDER": "openai"},
			notWant:     []string{"remove plaintext credentials", "ai_provder", "PVTR_AI_PROVIDER"},
		},
		{
			name:     "a clean configuration stays quiet",
			document: "ai_provider: openai\nai_model: model\n",
			notWant:  []string{"remove plaintext credentials", "ignoring unrecognized", "PVTR_AI_PROVIDER"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadConfig(t, tt.document)
			for name, value := range tt.environment {
				t.Setenv(name, value)
			}
			if tt.loglevel != "" {
				viper.Set("loglevel", tt.loglevel)
			}
			viper.Set("write", false)

			output := captureStderr(t, func() { NewConfig(nil) })

			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("diagnostics missing %q; operators would not see it.\ngot: %s", want, output)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(output, notWant) {
					t.Errorf("diagnostics unexpectedly contain %q.\ngot: %s", notWant, output)
				}
			}
		})
	}
}

// The diagnostics logger must not become a second channel that leaks the value
// it is warning about.
func TestNewConfig_DiagnosticsDoNotEmitTheCredential(t *testing.T) {
	loadConfig(t, "ai_provider: openai\nai_model: model\nai_api_key: super-secret-literal\n")
	viper.Set("write", false)

	output := captureStderr(t, func() { NewConfig(nil) })

	if !strings.Contains(output, "remove plaintext credentials") {
		t.Fatalf("expected the plaintext-credential warning, got: %s", output)
	}
	if strings.Contains(output, "super-secret-literal") {
		t.Errorf("diagnostics leaked the credential value: %s", output)
	}
}
