package config

import (
	"errors"
	"fmt"
	"maps"
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

// aiRecognizedKeys is every ai_* setting this package reads. Anything else
// carrying the prefix is ignored silently by the resolver, which is the exact
// failure mode explicit enablement was meant to remove: `ai_provder` leaves AI
// quietly off, and `ai_skipp` leaves it quietly on. reportIgnoredAISettings
// turns both into a warning.
var aiRecognizedKeys = []string{
	"ai_provider",
	"ai_model",
	"ai_base_url",
	"ai_timeout",
	"ai_max_tokens",
	"ai_skip",
	"ai_api_key",
	"ai_api_key_env",
}

// aiConfigOnlyKeys are recognized settings with no environment spelling, so a
// PVTR_ variable naming one is silently ignored and worth the same warning.
var aiConfigOnlyKeys = []string{"ai_api_key_env"}

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
	credentialSource, credentialErr := applyAICredential(resolved, sources)

	if !aiEnabled(resolved) {
		return nil
	}
	return errors.Join(
		requireUnambiguousFileConfig(sources.file, "ai_skip"),
		credentialErr,
		requireCredentialSafeBaseURL(sources, credentialSource),
		requireNamedCredentialForConfiguredBaseURL(sources, credentialSource),
	)
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
func applyAICredential(resolved map[string]interface{}, sources aiSettingSources) (aiCredentialSource, error) {
	delete(resolved, "ai_api_key")
	delete(resolved, "ai_api_key_env")

	// Unlike the ai_api_key literals below, a blank ai_api_key_env is not
	// treated as an unfilled placeholder to skip past. Naming a variable is an
	// explicit choice of account or tenant, so getting the name wrong has to
	// fail loudly rather than quietly spend whatever lower-priority credential
	// happens to be lying around. The AI client reports the empty name.
	if value, found := sources.target["ai_api_key_env"]; found {
		resolved["ai_api_key_env"] = value
		return aiCredentialNamedVariable, nil
	}
	if value, found := aiEnvironmentValue("ai_api_key"); found {
		resolved["ai_api_key"] = value
		return aiCredentialProcess, nil
	}
	if value, found := sources.shared["ai_api_key_env"]; found {
		resolved["ai_api_key_env"] = value
		return aiCredentialNamedVariable, nil
	}
	if err := requireUnambiguousFileConfig(sources.file, "ai_api_key_env"); err != nil {
		return aiCredentialNone, err
	}
	if value, found := aiRootShorthandValue(sources.file, "ai_api_key_env"); found {
		resolved["ai_api_key_env"] = value
		return aiCredentialNamedVariable, nil
	}
	// An empty literal is treated as "not configured" so that a placeholder
	// left in a target block falls through to the shared credential instead of
	// blanking it out. Non-string values fall through to the AI client, which
	// names the bad type.
	if value, found := sources.target["ai_api_key"]; found {
		if text, isString := value.(string); !isString || strings.TrimSpace(text) != "" {
			resolved["ai_api_key"] = value
			return aiCredentialConfigLiteral, nil
		}
	}
	if value, found := sources.shared["ai_api_key"]; found {
		if text, isString := value.(string); !isString || strings.TrimSpace(text) != "" {
			resolved["ai_api_key"] = value
			return aiCredentialConfigLiteral, nil
		}
	}
	if value, found := aiRootShorthandValue(sources.file, "ai_api_key"); found {
		resolved["ai_api_key"] = value
		return aiCredentialConfigLiteral, nil
	}
	return aiCredentialNone, nil
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
	text, found := os.LookupEnv(aiEnvironmentName(key))
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
	environment, present := os.LookupEnv(aiEnvironmentName(key))
	return isString && present && text == environment
}

// aiEnvironmentName renders the environment variable that carries a setting.
func aiEnvironmentName(key string) string {
	return "PVTR_" + strings.ToUpper(key)
}

// requireUnambiguousFileConfig refuses to proceed when an enabled AI run
// depends on a file setting that may have been masked by an environment
// variable of the same value, and the file was not loaded through ReadConfig or
// ReadInConfig so there is no captured copy to compare against.
//
// The ambiguity is unresolvable, which is also why it cannot be waved through
// when "the two agree": Viper reports the environment value and does not retain
// the file's, so PVTR_AI_SKIP=false looks identical whether the file said false
// or said true and was overridden. Guessing wrong runs AI against an explicit
// opt-out and bills a customer who asked not to be billed, so the run stops and
// tells the caller how to load configuration instead. The case where the
// environment value does not matter — PVTR_AI_SKIP=true, where true wins from
// any source — never reaches here, because applyAIPrecedenceRules returns
// before this call once AI is off.
func requireUnambiguousFileConfig(fileSettings *viper.Viper, key string) error {
	if fileSettings == nil && viper.InConfig(key) && isAIEnvironmentValue(key, viper.Get(key)) {
		return fmt.Errorf("%s is set in both the configuration file and %s, and the file value was not captured: unset the variable, or load configuration with config.ReadConfig or config.ReadInConfig to preserve file %s",
			key, aiEnvironmentName(key), key)
	}
	return nil
}

// aiEnabledByEnvironmentOnly reports whether PVTR_AI_PROVIDER turned AI on for
// a configuration that never asked for it. Enabled-but-invalid AI now stops a
// run at mobilization, so a leftover variable can fail a scan (or, with a model
// and credential also exported, bill one) that no config file mentions AI in.
// The variable stays authoritative, as it is for the other tunables, but the
// operator is told it is the one doing the enabling.
func aiEnabledByEnvironmentOnly(resolved map[string]interface{}, sources aiSettingSources) bool {
	if !aiEnabled(resolved) {
		return false
	}
	if _, found := aiEnvironmentValue("ai_provider"); !found {
		return false
	}
	if _, found := sources.target["ai_provider"]; found {
		return false
	}
	if _, found := sources.shared["ai_provider"]; found {
		return false
	}
	_, declaredInFile := aiFileSettingValue(sources.file, "ai_provider")
	return !declaredInFile
}

// AIEnablementHint returns operator guidance when the process environment
// selects the AI backend, and "" otherwise. Callers that report an AI
// configuration failure append it so an operator who did not write the setting
// can still find it.
func AIEnablementHint() string {
	name := aiEnvironmentName("ai_provider")
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%s=%q in the environment selects the AI backend and overrides any configured ai_provider; unset it or set ai_skip: true to turn AI off", name, value)
}

// ignoredAISettings lists ai_-prefixed settings the resolver does not read, so
// a misspelled key is reported instead of disappearing. Results are
// source-qualified: configuration keys by their config spelling, environment
// variables by their PVTR_ name.
//
// It is a warning rather than an error because the prefix is not reserved: a
// plugin may legitimately declare its own ai_-prefixed var, and a config
// written for a newer SDK should still run against an older one.
func ignoredAISettings(sources aiSettingSources) []string {
	ignored := make(map[string]struct{})
	collect := func(key string) {
		key = strings.ToLower(strings.TrimSpace(key))
		if strings.HasPrefix(key, "ai_") && !slices.Contains(aiRecognizedKeys, key) {
			ignored[key] = struct{}{}
		}
	}
	for key := range sources.target {
		collect(key)
	}
	for key := range sources.shared {
		collect(key)
	}
	for _, key := range aiRootLevelKeys(sources.file) {
		collect(key)
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		suffix, isPrefixed := strings.CutPrefix(name, "PVTR_")
		key := strings.ToLower(suffix)
		if !isPrefixed || !strings.HasPrefix(key, "ai_") {
			continue
		}
		if !slices.Contains(aiRecognizedKeys, key) || slices.Contains(aiConfigOnlyKeys, key) {
			ignored[name] = struct{}{}
		}
	}

	keys := slices.Collect(maps.Keys(ignored))
	slices.Sort(keys)
	return keys
}

// aiRootLevelKeys returns the keys written at the document root, where the
// flat AI shorthand lives. Nested keys are excluded: a typo under an
// unselected target is that target's problem, and the selected target's own
// vars arrive through sources.target.
func aiRootLevelKeys(fileSettings *viper.Viper) []string {
	settings := fileSettings
	if settings == nil {
		settings = viper.GetViper()
	}
	var keys []string
	for _, key := range settings.AllKeys() {
		if !strings.Contains(key, ".") {
			keys = append(keys, key)
		}
	}
	return keys
}

// aiCredentialSource records where the selected credential's value came from,
// which decides whether redirecting the endpoint may carry it along.
type aiCredentialSource int

const (
	aiCredentialNone aiCredentialSource = iota
	// aiCredentialNamedVariable is ai_api_key_env: the config names a
	// variable, but the value lives in the environment, alongside any
	// PVTR_AI_BASE_URL that redirects the endpoint.
	aiCredentialNamedVariable
	// aiCredentialProcess is PVTR_AI_API_KEY.
	aiCredentialProcess
	// aiCredentialConfigLiteral is an ai_api_key value written into the
	// config file at any level.
	aiCredentialConfigLiteral
)

// requireCredentialSafeBaseURL refuses to send a credential pinned in the
// config file to an endpoint chosen by PVTR_AI_BASE_URL.
//
// Endpoint and credential normally come from the same person. This is the one
// combination where they do not: ai_base_url is a tunable, so the environment
// outranks the file, and a variable left exported in a shell or CI job
// silently redirects a credential its author never agreed to share. Requiring
// https stops the network from reading it, but not the new endpoint.
//
// The other credential sources are exempt because their provenance already
// matches the endpoint's: PVTR_AI_API_KEY comes from the same environment, and
// ai_api_key_env names a variable whose value does too, so whoever redirected
// the endpoint also controls the credential.
func requireCredentialSafeBaseURL(sources aiSettingSources, credential aiCredentialSource) error {
	if credential != aiCredentialConfigLiteral {
		return nil
	}
	environmentURL, fromEnvironment := aiEnvironmentValue("ai_base_url")
	if !fromEnvironment {
		return nil
	}
	// A variable that merely restates the configured endpoint redirects
	// nothing. Unlike the ai_skip ambiguity, this comparison is sound: the
	// configured value is read from the target, the shared block, or the
	// captured file copy, none of which the environment can shadow.
	if configured, found := configuredAIBaseURL(sources); found && configured == environmentURL {
		return nil
	}
	return fmt.Errorf("%s redirects the endpoint while ai_api_key is set in configuration, which would send a config-pinned credential to an environment-chosen host: supply the credential for that endpoint with PVTR_AI_API_KEY or ai_api_key_env, or set ai_base_url in configuration instead",
		aiEnvironmentName("ai_base_url"))
}

// configuredAIBaseURL returns the endpoint the configuration itself declares,
// ignoring the environment. The root tier is consulted only when the file copy
// was captured, because without it Viper cannot distinguish a file value from
// the environment variable shadowing it, and guessing "they agree" would wave
// through the redirect this check exists to catch.
func configuredAIBaseURL(sources aiSettingSources) (interface{}, bool) {
	if value, found := sources.target["ai_base_url"]; found {
		return value, true
	}
	if value, found := sources.shared["ai_base_url"]; found {
		return value, true
	}
	if sources.file == nil {
		return nil, false
	}
	return aiFileSettingValue(sources.file, "ai_base_url")
}

// requireNamedCredentialForConfiguredBaseURL refuses to let a configuration
// file capture PVTR_AI_API_KEY for an endpoint that file chose.
//
// This is the mirror of requireCredentialSafeBaseURL. PVTR_AI_API_KEY is
// exported once and then applies to every later run, so a configuration that
// declares ai_base_url silently borrows a credential the operator never paired
// with that endpoint. Privateer searches the working directory ahead of
// ~/.privateer, so running inside an untrusted repository is enough for its
// config.yml to make that choice on the operator's behalf.
//
// Naming the variable with ai_api_key_env is how a configuration says yes: the
// pairing is then written down where the operator can read it, instead of
// being inferred from whatever happens to be exported.
func requireNamedCredentialForConfiguredBaseURL(sources aiSettingSources, credential aiCredentialSource) error {
	if credential != aiCredentialProcess {
		return nil
	}
	// Naming a variable is how a configuration states which credential it
	// expects. It counts as consent even when a higher rung of the ladder
	// supplies the value, because the operator can read the pairing in the
	// file either way.
	if aiConfigDeclaresNamedCredential(sources) {
		return nil
	}
	// The environment outranks the file for tunables, so once it supplies the
	// endpoint the configured value is not the one in use. Endpoint and
	// credential then share a provenance and nothing crosses a boundary.
	if _, fromEnvironment := aiEnvironmentValue("ai_base_url"); fromEnvironment {
		return nil
	}
	if _, found := configuredAIBaseURL(sources); !found {
		return nil
	}
	return fmt.Errorf("ai_base_url is set in configuration while the credential comes from %s, which would send an environment credential to a host the configuration chose: name the variable in configuration with ai_api_key_env to pair them deliberately, or select the endpoint with %s instead",
		aiEnvironmentName("ai_api_key"), aiEnvironmentName("ai_base_url"))
}

// aiConfigDeclaresNamedCredential reports whether the configuration names an
// environment variable for the credential at any tier.
func aiConfigDeclaresNamedCredential(sources aiSettingSources) bool {
	if _, found := sources.target["ai_api_key_env"]; found {
		return true
	}
	if _, found := sources.shared["ai_api_key_env"]; found {
		return true
	}
	if sources.file == nil {
		return false
	}
	_, found := aiFileSettingValue(sources.file, "ai_api_key_env")
	return found
}
