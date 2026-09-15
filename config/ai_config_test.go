package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestNewConfig_DoesNotMutateGlobalVarsBetweenTargets(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(`
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
`)); err != nil {
		t.Fatal(err)
	}
	viper.Set("target", "first")
	first := NewConfig(nil)
	viper.Set("target", "second")
	second := NewConfig(nil)
	if second.GetString("owner") != "shared-owner" || second.GetBool("ai_skip") ||
		second.GetString("ai_api_key_env") != "" || second.GetString("ai_api_key") != "shared-literal" {
		t.Fatal("first target settings contaminated the second target")
	}
	if first.GetString("owner") != "first-owner" || !first.GetBool("ai_skip") ||
		first.GetString("ai_api_key_env") != "FIRST_TARGET_KEY" {
		t.Fatal("resolving the second target changed the first config")
	}
	if viper.GetStringMap("vars")["owner"] != "shared-owner" {
		t.Fatal("NewConfig mutated Viper's global vars")
	}
}

func TestNewConfig_GlobalVarsSkipCannotBeOverriddenByTargetFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PVTR_AI_SKIP", "false")
	viper.Set("vars.ai_skip", true)
	viper.Set("targets.repo.vars.ai_skip", false)
	viper.Set("target", "repo")
	cfg := NewConfig(nil)
	if !cfg.GetBool("ai_skip") {
		t.Fatal("target false masked global vars ai_skip:true")
	}
}
