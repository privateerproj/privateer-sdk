package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// targetIsolationConfig gives one target its own AI settings and leaves the
// other inheriting the shared block, so a resolution that wrote back into
// shared state would show up as the second target seeing the first's values.
const targetIsolationConfig = `
vars:
  owner: shared-owner
  ai_skip: false
ai_provider: openai
ai_model: model
ai_api_key: shared-literal
targets:
  first:
    vars:
      owner: first-owner
      ai_skip: true
      ai_api_key_env: FIRST_TARGET_KEY
    policy: {catalogs: [catalog], applicability: [all]}
  second:
    policy: {catalogs: [catalog], applicability: [all]}
`

func loadTargetIsolationConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(targetIsolationConfig)); err != nil {
		t.Fatal(err)
	}
}

func configForTarget(t *testing.T, name string) Config {
	t.Helper()
	viper.Set("target", name)
	return NewConfig(nil)
}

func TestNewConfig_TargetSettingsDoNotLeakIntoTheNextTarget(t *testing.T) {
	loadTargetIsolationConfig(t)

	configForTarget(t, "first")
	second := configForTarget(t, "second")

	if got := second.GetString("owner"); got != "shared-owner" {
		t.Errorf("owner = %q, want the shared value %q; the first target's vars leaked", got, "shared-owner")
	}
	if second.GetBool("ai_skip") {
		t.Error("ai_skip = true, want false; the first target's opt-out leaked")
	}
	if got := second.GetString("ai_api_key_env"); got != "" {
		t.Errorf("ai_api_key_env = %q, want empty; the first target's named credential leaked", got)
	}
	if got := second.GetString("ai_api_key"); got != "shared-literal" {
		t.Errorf("ai_api_key = %q, want the shared literal %q", got, "shared-literal")
	}
}

func TestNewConfig_ResolvingATargetDoesNotDisturbAnEarlierConfig(t *testing.T) {
	loadTargetIsolationConfig(t)

	first := configForTarget(t, "first")
	configForTarget(t, "second")

	if got := first.GetString("owner"); got != "first-owner" {
		t.Errorf("owner = %q, want %q; resolving the second target rewrote the first config", got, "first-owner")
	}
	if !first.GetBool("ai_skip") {
		t.Error("ai_skip = false, want true; resolving the second target cleared the first target's opt-out")
	}
	if got := first.GetString("ai_api_key_env"); got != "FIRST_TARGET_KEY" {
		t.Errorf("ai_api_key_env = %q, want %q; resolving the second target replaced the first target's credential", got, "FIRST_TARGET_KEY")
	}
}

func TestNewConfig_DoesNotMutateViperGlobalVars(t *testing.T) {
	loadTargetIsolationConfig(t)

	configForTarget(t, "first")

	globalVars := viper.GetStringMap("vars")
	if got := globalVars["owner"]; got != "shared-owner" {
		t.Errorf("viper vars.owner = %v, want %q; NewConfig wrote a target value back into the shared block", got, "shared-owner")
	}
	if got := globalVars["ai_skip"]; got != false {
		t.Errorf("viper vars.ai_skip = %v, want false; NewConfig wrote a target value back into the shared block", got)
	}
	if _, leaked := globalVars["ai_api_key_env"]; leaked {
		t.Error("viper vars gained ai_api_key_env; NewConfig wrote a target value back into the shared block")
	}
}

func TestNewConfig_SharedSkipCannotBeUndoneByATargetFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_SKIP", "false")
	viper.Set("vars.ai_skip", true)
	viper.Set("targets.repo.vars.ai_skip", false)
	viper.Set("target", "repo")
	cfg := NewConfig(nil)
	if !cfg.GetBool("ai_skip") {
		t.Fatal("ai_skip = false, want true; a narrower target false undid the shared opt-out")
	}
}
