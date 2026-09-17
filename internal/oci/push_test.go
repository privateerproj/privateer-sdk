package oci

import (
	"context"
	"testing"

	"github.com/gemaraproj/grc-store-clientkit/hub"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// A RegistryToken builds a credentialed client that resolves the token for the
// host; absent it, the client stays the anonymous default (backward-compat).
func TestNewPluginRepository_RegistryTokenWiring(t *testing.T) {
	// With a token: the resolved credential carries it as AccessToken.
	repo, err := newPluginRepository(PushOptions{RegistryHost: "oci.example", RegistryToken: "reg-tok"}, "acme/hello")
	if err != nil {
		t.Fatal(err)
	}
	ac, ok := repo.Client.(*auth.Client)
	if !ok {
		t.Fatalf("expected *auth.Client for a token push, got %T", repo.Client)
	}
	cred, err := ac.Credential(context.Background(), "oci.example")
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "reg-tok" {
		t.Errorf("credential AccessToken = %q, want reg-tok", cred.AccessToken)
	}

	// Without a token: anonymous default client (not a credentialed auth.Client
	// carrying a token).
	anon, err := newPluginRepository(PushOptions{RegistryHost: "oci.example"}, "acme/hello")
	if err != nil {
		t.Fatal(err)
	}
	if anon.Client == nil {
		t.Error("anonymous push should still set an explicit client")
	}
}

func TestSplitCoordinate(t *testing.T) {
	cases := []struct {
		in     string
		ns, id string
		ok     bool
	}{
		{"ossf/pvtr-github-repo-scanner", "ossf", "pvtr-github-repo-scanner", true},
		{"finos-ccc/ccc-evaluator", "finos-ccc", "ccc-evaluator", true},
		{"  ossf/x  ", "ossf", "x", true},
		{"noslash", "", "", false},
		{"a/b/c", "", "", false}, // plugin_id must not contain a slash
		{"/x", "", "", false},
		{"x/", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		ns, id, ok := SplitCoordinate(c.in)
		if ok != c.ok || ns != c.ns || id != c.id {
			t.Errorf("SplitCoordinate(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, ns, id, ok, c.ns, c.id, c.ok)
		}
	}
}

func TestNewPluginRepository_PluginsPathAndScheme(t *testing.T) {
	repo, err := newPluginRepository(PushOptions{RegistryHost: "localhost:5050", PlainHTTP: true}, "ossf/pvtr-github-repo-scanner")
	if err != nil {
		t.Fatalf("newPluginRepository: %v", err)
	}
	// The reserved (OCI-spec-valid) `plugins` segment must be in the repo path.
	got := repo.Reference.Repository
	want := "ossf/plugins/pvtr-github-repo-scanner"
	if got != want {
		t.Errorf("repository path = %q, want %q", got, want)
	}
	if repo.Reference.Registry != "localhost:5050" {
		t.Errorf("registry = %q", repo.Reference.Registry)
	}
	if !repo.PlainHTTP {
		t.Error("PlainHTTP should be set for the dev override")
	}
}

// Guard: the reserved segment must parse cleanly through oras-go's real
// reference string parser — the whole point of the _plugins → plugins change.
// A regression to a leading-underscore (or otherwise OCI-invalid) segment would
// fail remote.NewRepository before any network call (mirrors the hub's
// TestReservedPluginSegmentIsOCIValid).
func TestNewPluginRepository_SegmentIsOCIValid(t *testing.T) {
	if _, err := newPluginRepository(PushOptions{RegistryHost: "oci.grc.store"}, "finos-ccc/ccc-evaluator"); err != nil {
		t.Fatalf("plugins-segment coordinate must parse via remote.NewRepository, got: %v", err)
	}
}

func TestNewPluginRepository_Validation(t *testing.T) {
	if _, err := newPluginRepository(PushOptions{RegistryHost: ""}, "ossf/x"); err == nil {
		t.Error("expected error for empty registry host")
	}
	if _, err := newPluginRepository(PushOptions{RegistryHost: "h"}, "noslash"); err == nil {
		t.Error("expected error for invalid coordinate")
	}
}

// The registry repository path has two independent definitions: pluginRepoPath
// builds the push reference here, and clientkit's hub.PluginRepository builds the
// token scope and the sync body. The hub compares them byte-for-byte, so a drift
// between the two would mint for one repo and push to another. Nothing but this
// test pins them together.
func TestPluginRepoPath_MatchesClientkit(t *testing.T) {
	for _, c := range []struct{ ns, id string }{
		{"acme", "hello"},
		{"acme-org", "my.plugin_v2"},
	} {
		if got, want := pluginRepoPath(c.ns, c.id), hub.PluginRepository(c.ns, c.id); got != want {
			t.Errorf("pluginRepoPath(%q, %q) = %q, clientkit hub.PluginRepository = %q", c.ns, c.id, got, want)
		}
	}
}
