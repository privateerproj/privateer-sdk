package oci

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// withEnvAwareViper resets viper and configures it exactly as command.ReadConfig
// does (PVTR_ env prefix, AutomaticEnv, "-"→"_" key replacer), so a test can
// exercise the real env→config→default resolution layering that HubURL relies on.
// It cannot call command.ReadConfig directly: command imports both config and
// internal/oci, so an oci test importing command would be an import cycle.
func withEnvAwareViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("PVTR")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func TestHubURL_Default(t *testing.T) {
	viper.Reset() // no stale hub-url from another test's viper.Set
	t.Cleanup(viper.Reset)
	t.Setenv(hubURLEnv, "")
	if got := HubURL(); got != DefaultHubURL {
		t.Errorf("expected default hub URL %q, got %q", DefaultHubURL, got)
	}
}

func TestHubURL_Override(t *testing.T) {
	t.Setenv(hubURLEnv, "http://localhost:8088")
	if got := HubURL(); got != "http://localhost:8088" {
		t.Errorf("expected override, got %q", got)
	}
}

func TestHubURL_TrimsTrailingSlash(t *testing.T) {
	t.Setenv(hubURLEnv, "http://localhost:8088/")
	if got := HubURL(); got != "http://localhost:8088" {
		t.Errorf("expected trailing slash trimmed, got %q", got)
	}
}

// The hub URL is a first-class config key: a value set in config.yml (here
// modeled by viper.Set, which is how a loaded config surfaces) is honored and
// trailing-slash-normalized, even when no PVTR_HUB_URL env var is present.
// TestHubURL_FromViperConfig covers the highest-precedence path: an explicit
// viper.Set (the override tier). This is NOT the config-file tier — a loaded
// config.yml surfaces below env, and that path is covered separately by
// TestHubURL_FromConfigFile / TestHubURL_EnvBeatsConfigFile. Kept distinct so the
// precedence model stays unambiguous.
func TestHubURL_FromViperConfig(t *testing.T) {
	t.Setenv(hubURLEnv, "") // ensure the env fallback can't supply the value
	viper.Set(hubURLKey, "http://config.example:9000/")
	defer viper.Reset()
	if got := HubURL(); got != "http://config.example:9000" {
		t.Errorf("expected hub URL from viper override, got %q", got)
	}
}

// hub-url read from an actual config.yml body, through viper's config layer (not
// the programmatic Set override tier). This is the real "first-class config.yml"
// path the feature promises, and the env var is absent here so the config value
// must be what HubURL returns.
func TestHubURL_FromConfigFile(t *testing.T) {
	withEnvAwareViper(t)
	t.Setenv(hubURLEnv, "")
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader("hub-url: http://from-file:9000\n")); err != nil {
		t.Fatalf("reading in-memory config: %v", err)
	}
	if got := HubURL(); got != "http://from-file:9000" {
		t.Errorf("expected hub URL from config file, got %q", got)
	}
}

// Precedence: when both config.yml and the PVTR_HUB_URL env var set the hub URL,
// the env var wins (viper's standard env-over-file ordering). This is the CI
// scenario — an operator's config.yml value must not silently override a hub the
// pipeline injects via env. Asserting it guards the precedence against future
// regressions in the viper wiring.
func TestHubURL_EnvBeatsConfigFile(t *testing.T) {
	withEnvAwareViper(t)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader("hub-url: http://from-file:9000\n")); err != nil {
		t.Fatalf("reading in-memory config: %v", err)
	}
	t.Setenv(hubURLEnv, "http://from-env:8088")
	if got := HubURL(); got != "http://from-env:8088" {
		t.Errorf("expected env to win over config file, got %q", got)
	}
}

func TestNewClient_UsesConfiguredHub(t *testing.T) {
	t.Setenv(hubURLEnv, "http://localhost:8088")
	if got := NewClient().BaseURL(); got != "http://localhost:8088" {
		t.Errorf("expected client base http://localhost:8088, got %q", got)
	}
}

// Hub-URL resolution: the viper key, the PVTR_ env prefix, and the default.
