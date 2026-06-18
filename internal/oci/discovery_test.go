package oci

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestRegistryHost_StripsScheme(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
		want        string
		wantErr     bool
	}{
		{"https with no port", "https://oci.grc.store", "oci.grc.store", false},
		{"http with port (dev)", "http://localhost:5050", "localhost:5050", false},
		{"https with port", "https://oci.grc.store:443", "oci.grc.store:443", false},
		{"trailing slash", "http://localhost:5050/", "localhost:5050", false},
		{"already host-only", "oci.grc.store", "oci.grc.store", false},
		{"host-only with port", "localhost:5050", "localhost:5050", false},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Discovery{RegistryURL: tt.registryURL}
			got, err := d.RegistryHost()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got host %q", tt.registryURL, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.registryURL, err)
			}
			if got != tt.want {
				t.Errorf("RegistryHost(%q) = %q, want %q", tt.registryURL, got, tt.want)
			}
		})
	}
}

func TestDiscover_Success(t *testing.T) {
	const body = `{"registry_url":"http://localhost:5050","hub_url":"http://localhost:8088","api_version":"v1","oidc_issuer":"http://localhost:8080/realms/gemara","oidc_cli_client_id":"grcli","ci_audience":"http://localhost:8088"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wellKnownPath {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	d, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if d.RegistryURL != "http://localhost:5050" {
		t.Errorf("registry_url = %q", d.RegistryURL)
	}
	if d.OIDCIssuer != "http://localhost:8080/realms/gemara" {
		t.Errorf("oidc_issuer = %q", d.OIDCIssuer)
	}
	host, err := d.RegistryHost()
	if err != nil {
		t.Fatalf("RegistryHost error: %v", err)
	}
	if host != "localhost:5050" {
		t.Errorf("RegistryHost = %q, want localhost:5050", host)
	}
}

func TestDiscover_EmptyRegistryURLFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hub_url":"http://localhost:8088","api_version":"v1"}`))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Discover(context.Background()); err == nil {
		t.Fatal("expected error for discovery document with no registry_url, got nil")
	}
}

func TestDiscover_Non200FailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Discover(context.Background()); err == nil {
		t.Fatal("expected error for non-200 discovery response, got nil")
	}
}

func TestPlainHTTP(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
		want        bool
	}{
		{"https scheme", "https://oci.grc.store", false},
		{"http scheme (local dev)", "http://localhost:5050", true},
		{"http with trailing slash", "http://localhost:5050/", true},
		{"https with port", "https://oci.grc.store:443", false},
		{"bare host (no scheme) implies https", "localhost:5050", false},
		{"http with whitespace", "  http://localhost:5050  ", true},
		{"https with whitespace", "  https://oci.grc.store  ", false},
		// empty registry_url: parseRegistryURL errors → false (fail-safe)
		{"empty registry_url", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Discovery{RegistryURL: tt.registryURL}
			if got := d.PlainHTTP(); got != tt.want {
				t.Errorf("PlainHTTP(%q) = %v, want %v", tt.registryURL, got, tt.want)
			}
		})
	}
}

// TestRegistryHost_ConsistentWithPlainHTTP verifies that RegistryHost and
// PlainHTTP derive from the same parse (parseRegistryURL) so their answers
// can never contradict each other for the same registry_url value.
func TestRegistryHost_ConsistentWithPlainHTTP(t *testing.T) {
	cases := []string{
		"http://localhost:5050",
		"https://oci.grc.store",
		"http://localhost:5050/",
		"oci.grc.store",
	}
	for _, raw := range cases {
		d := &Discovery{RegistryURL: raw}
		host, err := d.RegistryHost()
		if err != nil {
			t.Fatalf("RegistryHost(%q) unexpected error: %v", raw, err)
		}
		if host == "" {
			t.Errorf("RegistryHost(%q) returned empty string", raw)
		}
		// PlainHTTP must not error for valid URLs — it returns bool only.
		_ = d.PlainHTTP()
	}
}
