// This file is an external test package so it can import ai/provider, which
// imports config and therefore cannot be imported from the config package's
// internal tests. Resolution tests belong next to the code, but asserting that
// AI is genuinely skipped requires the consumer that acts on ai_skip.
package config_test

import (
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/ai/provider"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

// mixedOptOutConfig models the real motivation for a per-target opt-out: one
// run covering repositories with different data-handling rules. Both targets
// are fully configured for AI and differ only in ai_skip, so an assertion that
// one is disabled and the other enabled can only be explained by ai_skip.
const mixedOptOutConfig = `
vars:
  ai_provider: openai
  ai_model: gpt-4o-mini
  ai_api_key: shared-literal-key
targets:
  public-repo:
    vars:
      ai_skip: false
    policy: {catalogs: [catalog], applicability: [all]}
  confidential-repo:
    vars:
      ai_skip: true
    policy: {catalogs: [catalog], applicability: [all]}
`

func aiConfigForTarget(t *testing.T, name string) (provider.Config, bool, error) {
	t.Helper()
	viper.Set("target", name)
	cfg := config.NewConfig(nil)
	if cfg.Error != nil {
		t.Fatalf("target %q: NewConfig returned %v, want a usable config", name, cfg.Error)
	}
	return provider.ConfigFromSDKConfig(cfg)
}

// TestAISkipDisablesOnlyTheOptedOutTarget checks the behavior ai_skip exists
// for, not merely that the resolved var holds the expected value: the opted-out
// target must produce no usable AI configuration, and its neighbor in the same
// run must still get one.
func TestAISkipDisablesOnlyTheOptedOutTarget(t *testing.T) {
	// Resolution order must not matter, so run it both ways.
	for _, order := range [][]string{
		{"public-repo", "confidential-repo"},
		{"confidential-repo", "public-repo"},
	} {
		t.Run(strings.Join(order, "-then-"), func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.SetConfigType("yaml")
			if err := viper.ReadConfig(strings.NewReader(mixedOptOutConfig)); err != nil {
				t.Fatal(err)
			}

			results := make(map[string]provider.Config, len(order))
			enabled := make(map[string]bool, len(order))
			for _, name := range order {
				aiConfig, configured, err := aiConfigForTarget(t, name)
				if err != nil {
					t.Fatalf("target %q: ConfigFromSDKConfig returned %v, want no error", name, err)
				}
				results[name] = aiConfig
				enabled[name] = configured
			}

			if enabled["confidential-repo"] {
				t.Error("confidential-repo: AI is enabled despite ai_skip: true; the opt-out did not take effect")
			}
			if got := results["confidential-repo"]; got != (provider.Config{}) {
				t.Errorf("confidential-repo: AI config = %+v, want the zero value; a skipped target must not carry provider settings", got)
			}

			if !enabled["public-repo"] {
				t.Fatal("public-repo: AI is disabled despite ai_skip: false; the neighbor's opt-out suppressed an opted-in target")
			}
			publicRepo := results["public-repo"]
			if publicRepo.Provider != provider.Provider("openai") {
				t.Errorf("public-repo: provider = %q, want %q", publicRepo.Provider, "openai")
			}
			if publicRepo.Model != "gpt-4o-mini" {
				t.Errorf("public-repo: model = %q, want %q", publicRepo.Model, "gpt-4o-mini")
			}
			if publicRepo.APIKey != "shared-literal-key" {
				t.Errorf("public-repo: api key = %q, want the shared literal", publicRepo.APIKey)
			}
		})
	}
}

// TestAISkipOptOutSurvivesAnEnvironmentOptIn covers the true-wins rule at the
// boundary that matters operationally: an opt-out written for a confidential
// repository must not be undone by an ambient PVTR_AI_SKIP=false in CI.
func TestAISkipOptOutSurvivesAnEnvironmentOptIn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_SKIP", "false")
	viper.SetConfigType("yaml")
	if err := config.ReadConfig(strings.NewReader(mixedOptOutConfig)); err != nil {
		t.Fatal(err)
	}

	_, configured, err := aiConfigForTarget(t, "confidential-repo")
	if err != nil {
		t.Fatalf("ConfigFromSDKConfig returned %v, want no error", err)
	}
	if configured {
		t.Error("AI is enabled; an environment opt-in overrode the target's ai_skip: true")
	}

	if _, configured, err = aiConfigForTarget(t, "public-repo"); err != nil {
		t.Fatalf("ConfigFromSDKConfig returned %v, want no error", err)
	}
	if !configured {
		t.Error("AI is disabled for the opted-in target; true-wins was applied too broadly")
	}
}
