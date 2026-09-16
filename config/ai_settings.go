package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

const aiAPIKeyConfigWarning = "ai_api_key is declared in configuration; remove plaintext credentials and use ai_api_key_env for per-target credentials or PVTR_AI_API_KEY for a shared credential"

// aiTunableKeys are the AI settings an operator is expected to retune per run:
// which backend, which model, where it lives, and how much it may spend. They
// carry no policy or secret meaning, so a per-run environment override is a
// legitimate way to redirect a scan (say, at a staging endpoint in CI) without
// editing a checked-in config file.
var aiTunableKeys = []string{
	"ai_provider",
	"ai_model",
	"ai_base_url",
	"ai_timeout",
	"ai_max_tokens",
}

// aiSettingSources represents scopes in a config such as:
//
// vars: # shared
//
//	ai_provider: openai
//
// targets:
//
//	example:
//	  vars: # target
//	    ai_model: gpt-4o-mini
//
// file preserves the config values from before environment overrides.
type aiSettingSources struct {
	target map[string]interface{}
	shared map[string]interface{}
	file   *viper.Viper
}

// applyAIPrecedenceRules resolves tunables, ai_skip, and credentials (ai_api_key, ai_api_key_env, or PVTR_AI_API_KEY).
// Errors apply only when ai_provider is nonblank and ai_skip is not true.
//
// Three different precedence rules apply, because the three groups of settings
// answer to different people:
//
//   - Tunables (aiTunableKeys) take the environment first. These are the knobs
//     an operator is meant to turn per run, so an explicit environment value is
//     treated as the more recent instruction and overrides the file.
//
//   - ai_skip is true-wins across every source. Opting out of AI is a cost,
//     privacy, or policy decision, and the person who made it is usually not
//     the person running the scan. A narrower scope must never silently
//     re-enable AI, so a target block cannot undo a repository-wide opt-out and
//     a stray environment variable cannot undo either.
//
//   - API keys referenced by ai_api_key_env take priority over plaintext ai_api_key values.
//
// Helpers named apply* write their result into resolved; helpers named select*
// or *Value compute a value and hand it back.
func applyAIPrecedenceRules(resolved map[string]interface{}, sources aiSettingSources) error {
	applyAITunables(resolved, sources)
	if value, found := selectEffectiveAISkipValue(sources); found {
		resolved["ai_skip"] = value
	}

	// Always select the credential so consumers see one resolved source, but
	// hold any complaint until we know AI is enabled.
	credentialErr := applyAICredential(resolved, sources)

	if !aiEnabled(resolved) {
		return nil
	}
	return errors.Join(requireUnambiguousFileConfig(sources.file, "ai_skip"), credentialErr)
}

// applyAITunables fills in the tunables the target did not set for itself.
func applyAITunables(resolved map[string]interface{}, sources aiSettingSources) {
	for _, key := range aiTunableKeys {
		if value, found := aiEnvironmentValue(key); found {
			resolved[key] = value
			continue
		}
		if _, alreadySet := resolved[key]; alreadySet {
			continue
		}
		// Accept the root-level shorthand (`ai_model: gpt-4o`) as a last
		// resort so a single-target config need not nest a vars block.
		if value, found := aiRootShorthandValue(sources.file, key); found {
			resolved[key] = value
		}
	}
}

// selectEffectiveAISkipValue selects ai_skip across all sources; true wins.
// Malformed values (such as "no", "maybe") are preserved for validation.
func selectEffectiveAISkipValue(sources aiSettingSources) (interface{}, bool) {
	declared := make([]interface{}, 0, 4)
	if value, found := aiEnvironmentValue("ai_skip"); found {
		declared = append(declared, value)
	}
	if value, found := sources.target["ai_skip"]; found {
		declared = append(declared, value)
	}
	if value, found := sources.shared["ai_skip"]; found {
		declared = append(declared, value)
	}
	if value, found := aiRootShorthandValue(sources.file, "ai_skip"); found {
		declared = append(declared, value)
	}
	// Consult the file directly as well: an environment PVTR_AI_SKIP=false
	// masks the file value in Viper, and the file's opt-out has to survive it.
	if value, found := aiFileSettingValue(sources.file, "ai_skip"); found {
		declared = append(declared, value)
	}

	for _, value := range declared {
		if skip, isBool := value.(bool); isBool && skip {
			return true, true
		}
	}
	for _, value := range declared {
		if _, isBool := value.(bool); !isBool {
			return value, true
		}
	}
	if len(declared) > 0 {
		return false, true
	}
	return nil, false
}

// applyAICredential picks exactly one credential source and writes it to
// resolved, removing the others so no consumer has to re-derive the ladder:
//
//  1. the target's ai_api_key_env
//  2. PVTR_AI_API_KEY
//  3. the shared ai_api_key_env, then the root-level one
//  4. the target's ai_api_key literal, then the shared and root-level ones
//
// Two rules shape that order. First, every source that keeps the credential out
// of the config file outranks the plaintext ai_api_key literals written into
// it: a literal is a credential headed for version control and log output, so a
// config carrying both is read as a half-finished migration and the safe source
// wins, which makes deleting the literal a no-op. Second, within each group the
// narrower scope wins, except that PVTR_AI_API_KEY outranks the config-wide
// named variables: a target naming its own variable is selecting a specific
// account or tenant and must not be overridden, while a variable named for the
// whole config is only a default that the operator running the scan may replace.
//
// A named variable that turns out to be unset is reported by the AI client
// rather than skipped over. Falling through would spend a different account's
// quota than the operator named.
func applyAICredential(resolved map[string]interface{}, sources aiSettingSources) error {
	delete(resolved, "ai_api_key")
	delete(resolved, "ai_api_key_env")

	if value, found := sources.target["ai_api_key_env"]; found {
		resolved["ai_api_key_env"] = value
		return nil
	}
	if value, found := aiEnvironmentValue("ai_api_key"); found {
		resolved["ai_api_key"] = value
		return nil
	}
	if value, found := sources.shared["ai_api_key_env"]; found {
		resolved["ai_api_key_env"] = value
		return nil
	}
	if err := requireUnambiguousFileConfig(sources.file, "ai_api_key_env"); err != nil {
		return err
	}
	if value, found := aiRootShorthandValue(sources.file, "ai_api_key_env"); found {
		resolved["ai_api_key_env"] = value
		return nil
	}
	// An empty literal is treated as "not configured" so that a placeholder
	// left in a target block falls through to the shared credential instead of
	// blanking it out. Non-string values fall through to the AI client, which
	// names the bad type.
	if value, found := sources.target["ai_api_key"]; found {
		if text, isString := value.(string); !isString || strings.TrimSpace(text) != "" {
			resolved["ai_api_key"] = value
			return nil
		}
	}
	if value, found := sources.shared["ai_api_key"]; found {
		resolved["ai_api_key"] = value
		return nil
	}
	if value, found := aiRootShorthandValue(sources.file, "ai_api_key"); found {
		resolved["ai_api_key"] = value
	}
	return nil
}

// aiEnabled reports whether this run will actually call a model. ai_provider is
// the single enablement switch: naming a backend is the only way to turn AI on,
// so a half-written block of other ai_* keys stays dormant instead of
// surprising an operator with model calls they never asked for. A
// non-string or otherwise malformed provider still counts as enabled so that
// the AI client reports the mistake rather than silently disabling itself.
func aiEnabled(resolved map[string]interface{}) bool {
	if skip, _ := resolved["ai_skip"].(bool); skip {
		return false
	}
	provider, declared := resolved["ai_provider"]
	if !declared {
		return false
	}
	text, isString := provider.(string)
	return !isString || strings.TrimSpace(text) != ""
}

// hasConfigFileAIAPIKey reports whether a plaintext credential was written into
// the configuration file, in any of the places it may legally appear. It drives
// a warning rather than an error: the key still works, but it is one `git add`
// away from being published.
func hasConfigFileAIAPIKey(serviceName, targetsSectionKey string) bool {
	for _, key := range []string{"ai_api_key", "vars.ai_api_key"} {
		if viper.InConfig(key) {
			return true
		}
	}
	if serviceName != "" {
		return viper.InConfig(fmt.Sprintf("%s.%s.vars.ai_api_key", targetsSectionKey, serviceName))
	}
	return false
}

// aiEnvironmentValue reads an AI setting from the process environment under the
// PVTR_ prefix, typed to match what the setting means so downstream type checks
// see an int or a bool rather than the string the shell handed us.
//
// A variable set to whitespace counts as unset. Blank environment variables are
// routinely how CI expresses "no value supplied" for an unfilled template, and
// letting one through would override a working file setting with nothing.
func aiEnvironmentValue(key string) (interface{}, bool) {
	text, found := os.LookupEnv("PVTR_" + strings.ToUpper(key))
	text = strings.TrimSpace(text)
	if !found || text == "" {
		return nil, false
	}
	switch key {
	case "ai_max_tokens":
		if value, err := strconv.Atoi(text); err == nil {
			return value, true
		}
	case "ai_skip":
		if value, err := strconv.ParseBool(text); err == nil {
			return value, true
		}
	}
	// An unparseable value is passed through as text so the AI client can
	// name the setting and the bad value instead of silently ignoring it.
	return text, true
}

// aiRootShorthandValue reads an AI setting written at the document root
// (`ai_provider: openai`) rather than inside a vars block.
//
// Viper is asked first so that flags and other programmatic overrides are
// honored, but a value that merely echoes the environment is discarded and
// re-read from the file. Each caller folds the environment in at its own
// precedence tier, and accepting Viper's copy here would apply it a second time
// at the wrong tier: it would let PVTR_AI_SKIP=false outrank a file opt-out, and
// let a whitespace-only variable that aiEnvironmentValue deliberately ignored
// come back in through the side door.
func aiRootShorthandValue(fileSettings *viper.Viper, key string) (interface{}, bool) {
	if viper.IsSet(key) {
		value := viper.Get(key)
		if !isAIEnvironmentValue(key, value) {
			return value, true
		}
	}
	return aiFileSettingValue(fileSettings, key)
}

// aiFileSettingValue reads a setting from the configuration file alone,
// ignoring every other source.
func aiFileSettingValue(fileSettings *viper.Viper, key string) (interface{}, bool) {
	if fileSettings != nil {
		// AllKeys is used rather than InConfig because InConfig reports an
		// explicit `ai_skip: null` as absent, and an operator who wrote the key
		// out longhand deserves an error about the null instead of silence.
		if !slices.Contains(fileSettings.AllKeys(), key) {
			return nil, false
		}
		return fileSettings.Get(key), true
	}
	if !viper.InConfig(key) {
		return nil, false
	}
	return viper.Get(key), true
}

// isAIEnvironmentValue reports whether a value Viper returned is really just
// the environment variable showing through. Viper does not record where a value
// came from, so an exact match against the environment is the only signal
// available.
func isAIEnvironmentValue(key string, value interface{}) bool {
	text, isString := value.(string)
	environment, present := os.LookupEnv("PVTR_" + strings.ToUpper(key))
	return isString && present && text == environment
}

// requireUnambiguousFileConfig refuses to proceed when an enabled AI run
// depends on a file setting that may have been masked by an environment
// variable of the same value, and the file was not loaded through ReadConfig or
// ReadInConfig so there is no captured copy to compare against.
//
// The ambiguity is unresolvable: PVTR_AI_SKIP=false looks identical whether the
// file said false or said true and was overridden. Guessing wrong runs AI
// against an explicit opt-out and bills a customer who asked not to be billed,
// so the run stops and tells the caller how to load configuration instead.
func requireUnambiguousFileConfig(fileSettings *viper.Viper, key string) error {
	if fileSettings == nil && viper.InConfig(key) && isAIEnvironmentValue(key, viper.Get(key)) {
		return fmt.Errorf("load configuration with config.ReadConfig or config.ReadInConfig to preserve file %s", key)
	}
	return nil
}
