package publish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/shared"
)

// stubManifest returns a resolveManifest func that yields a fixed manifest,
// standing in for exec'ing a real plugin binary (so tests need no host-platform
// build in their dist).
func stubManifest(m pluginkit.PublishManifest) func(context.Context, []oci.PlatformBinary) (pluginkit.PublishManifest, error) {
	return func(context.Context, []oci.PlatformBinary) (pluginkit.PublishManifest, error) { return m, nil }
}

// acmeHelloManifest is the publish manifest a well-formed acme/hello plugin
// would emit — owner coordinate + one evaluated catalog — used to drive the
// ownership/push tests without a real plugin binary.
func acmeHelloManifest() pluginkit.PublishManifest {
	return pluginkit.PublishManifest{
		Coordinate: "acme/hello",
		License:    "Apache-2.0",
		Evaluates: []pluginkit.EvaluatesDeclaration{{
			Catalog:        "acme/example",
			CatalogVersion: "2026.01",
			RequirementIDs: []string{"R1"},
		}},
	}
}

func TestPublish_BadDistFailsBeforeNetwork(t *testing.T) {
	// A nonexistent dist dir must fail at the load step — before the plugin is
	// run or any hub discovery — so the error names the dist load.
	err := Publish(context.Background(), io.Discard, Params{DistDir: "/nonexistent/dist"})
	if err == nil {
		t.Fatal("expected error for missing dist dir")
	}
	if !strings.Contains(err.Error(), "GoReleaser build") {
		t.Errorf("expected a dist-load error, got %v", err)
	}
}

func TestPublish_NoCoordinateFromManifest(t *testing.T) {
	// A plugin that declares no coordinate cannot be published; the error must
	// name the coordinate, and it must surface before any hub interaction.
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(pluginkit.PublishManifest{}),
	})
	if err == nil {
		t.Fatal("expected an error when the plugin declares no coordinate")
	}
	if !strings.Contains(err.Error(), "coordinate") {
		t.Errorf("error should name the missing coordinate, got %v", err)
	}
}

func TestPublish_MissingEvaluatesFailsBeforePush(t *testing.T) {
	// A manifest with a coordinate but no evaluates must fail at preflight —
	// before discovery/push — with the "must declare what it evaluates" error.
	// No PVTR_HUB_URL is set, so reaching discovery would produce a different
	// (network) error; asserting the evaluates message proves we failed first.
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(pluginkit.PublishManifest{Coordinate: "acme/hello", License: "Apache-2.0"}),
	})
	if err == nil {
		t.Fatal("expected preflight to reject a manifest with no evaluates")
	}
	if !strings.Contains(err.Error(), "evaluates") {
		t.Errorf("error should name evaluates, got %v", err)
	}
}

func TestPublish_NoLicenseFailsBeforePush(t *testing.T) {
	// A manifest with a coordinate + evaluates but no license must fail at
	// preflight — before discovery/push — naming the license. No PVTR_HUB_URL is
	// set, so reaching discovery would be a different (network) error.
	m := acmeHelloManifest()
	m.License = ""
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(m),
	})
	if err == nil {
		t.Fatal("expected preflight to reject a manifest with no license")
	}
	if !strings.Contains(err.Error(), "license") {
		t.Errorf("error should name the missing license, got %v", err)
	}
}

func TestPublish_InvalidLicenseFailsBeforePush(t *testing.T) {
	// A well-formed-but-unknown SPDX id must fail the strict grcli gate before any
	// push, rather than being signed and pushed for the hub to reject.
	m := acmeHelloManifest()
	m.License = "Definitely-Not-A-License-9000"
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(m),
	})
	if err == nil {
		t.Fatal("expected preflight to reject an invalid SPDX license")
	}
	if !strings.Contains(err.Error(), "SPDX") {
		t.Errorf("error should name the SPDX validation failure, got %v", err)
	}
}

func TestPublish_RegistryPathRejectsInvalidLicense(t *testing.T) {
	// The --registry smoke path is exempt from the hub contract, but
	// canonicalization still applies (signed ⇒ canonical), so an invalid SPDX id
	// fails before any push — the registry host below is never reached.
	m := acmeHelloManifest()
	m.License = "Definitely-Not-A-License-9000"
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		Registry:        "http://127.0.0.1:1",
		resolveManifest: stubManifest(m),
	})
	if err == nil {
		t.Fatal("expected the --registry path to reject an invalid SPDX license")
	}
	if !strings.Contains(err.Error(), "SPDX") {
		t.Errorf("error should be the SPDX failure (not a network error), got %v", err)
	}
}

func TestPublish_RegistryPathAllowsEmptyLicense(t *testing.T) {
	// --registry is exempt from the license REQUIREMENT (it never /syncs). An empty
	// license must get PAST the gate and fail later at the push, NOT at the license.
	// 400 (not 5xx) so oras fails fast without retry backoff — the push just needs
	// to fail for a reason that is NOT the license.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	m := acmeHelloManifest()
	m.License = ""
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		Registry:        srv.URL,
		resolveManifest: stubManifest(m),
	})
	if err == nil {
		t.Fatal("expected a push failure against the stub registry")
	}
	if strings.Contains(err.Error(), "license") || strings.Contains(err.Error(), "SPDX") {
		t.Errorf("empty license must be allowed on the --registry path; the failure should be the push, got %v", err)
	}
}

// A non-plugin binary in the dist aborts publish in preflight — before any
// discovery, token mint, push, or sign.
func TestPublish_NonPluginRejectedBeforePush(t *testing.T) {
	pushHit := false
	hub := mockHub(t, []string{"pull", "push"}, &pushHit) // owner token, so only the marker check can stop it
	defer hub.Close()
	t.Setenv("PVTR_HUB_URL", hub.URL)
	t.Setenv("PVTR_TOKEN", "stub-upstream-bearer")

	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeNonPluginDist(t),
		resolveManifest: stubManifest(acmeHelloManifest()),
	})
	if err == nil {
		t.Fatal("expected a non-plugin binary to be rejected")
	}
	if !strings.Contains(err.Error(), "not a Privateer plugin") {
		t.Errorf("error should name the missing handshake marker, got: %v", err)
	}
	if pushHit {
		t.Error("a non-plugin must be rejected BEFORE any push")
	}
}

// The core requirement: a pull-only token (unowned namespace) aborts publish
// with the ownership error BEFORE any push or sign. No bytes land; no signing
// prompt.
func TestPublish_PullOnlyTokenAbortsBeforePush(t *testing.T) {
	pushHit := false
	hub := mockHub(t, []string{"pull"}, &pushHit)
	defer hub.Close()

	t.Setenv("PVTR_HUB_URL", hub.URL)
	t.Setenv("PVTR_TOKEN", "stub-upstream-bearer") // bypass the device-grant store

	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(acmeHelloManifest()),
	})
	if err == nil {
		t.Fatal("expected an ownership error for a pull-only token")
	}
	if !strings.Contains(err.Error(), "requires ownership of namespace") {
		t.Errorf("error should name the ownership requirement, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"acme"`) {
		t.Errorf("error should name the namespace acme, got: %v", err)
	}
	if pushHit {
		t.Error("push must NOT be attempted when the token lacks push (fail fast before push)")
	}
}

// An owner (pull,push) token gets past the ownership gate and proceeds to push
// (which then hits the mock zot — proving the gate let it through).
func TestPublish_OwnerTokenProceedsToPush(t *testing.T) {
	pushHit := false
	hub := mockHub(t, []string{"pull", "push"}, &pushHit)
	defer hub.Close()

	t.Setenv("PVTR_HUB_URL", hub.URL)
	t.Setenv("PVTR_TOKEN", "stub-upstream-bearer")

	// We only need to prove the ownership gate PASSED — i.e. push was REACHED.
	// The naive mock zot can't satisfy oras's full blob-upload protocol, so the
	// push itself errors AFTER the gate; that's fine. The assertions are: the
	// error is NOT the ownership error, and push was attempted.
	err := Publish(context.Background(), io.Discard, Params{
		DistDir:         writeMinimalDist(t),
		resolveManifest: stubManifest(acmeHelloManifest()),
		NoSync:          true,
	})
	if err != nil && strings.Contains(err.Error(), "requires ownership of namespace") {
		t.Fatalf("owner token must pass the ownership gate, but it was rejected: %v", err)
	}
	if !pushHit {
		t.Error("owner token should have proceeded to push (the gate should have let it through)")
	}
}

func TestParseRegistryOverride(t *testing.T) {
	cases := []struct {
		raw       string
		host      string
		plainHTTP bool
		wantErr   bool
	}{
		{"http://localhost:5000", "localhost:5000", true, false},
		{"https://ghcr.io/acme", "ghcr.io/acme", false, false},
		{"https://oci.grc.store/", "oci.grc.store", false, false},
		{"localhost:5000", "", false, true},  // no scheme
		{"ftp://localhost", "", false, true}, // bad scheme
		{"https://", "", false, true},        // no host
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			host, plain, err := parseRegistryOverride(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tc.host || plain != tc.plainHTTP {
				t.Errorf("got (%q, %v), want (%q, %v)", host, plain, tc.host, tc.plainHTTP)
			}
		})
	}
}

// writeMinimalDist creates a real GoReleaser-shaped dist (artifacts.json +
// metadata.json + one binary file) so LoadGoReleaserBuild + AssembleIndex
// succeed, letting the test reach the ownership check.
func writeMinimalDist(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	binDir := filepath.Join(dist, "p_linux_amd64_v1")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The binary must carry the go-plugin handshake marker, since publish now
	// rejects non-plugins in preflight (before the ownership/resolution logic
	// these tests exercise).
	hc := shared.GetHandshakeConfig()
	if err := os.WriteFile(filepath.Join(binDir, "hello"), []byte("ELF"+hc.MagicCookieKey+hc.MagicCookieValue), 0o755); err != nil {
		t.Fatal(err)
	}
	arts := []map[string]any{
		{
			"name": "hello", "path": "dist/p_linux_amd64_v1/hello",
			"goos": "linux", "goarch": "amd64", "type": "Binary",
			"extra": map[string]any{"Binary": "hello"},
		},
	}
	ab, _ := json.Marshal(arts)
	if err := os.WriteFile(filepath.Join(dist, "artifacts.json"), ab, 0o644); err != nil {
		t.Fatal(err)
	}
	mb, _ := json.Marshal(map[string]any{"version": "0.1.0", "project_name": "hello"})
	if err := os.WriteFile(filepath.Join(dist, "metadata.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	return dist
}

// writeNonPluginDist is writeMinimalDist but with a binary that LACKS the
// handshake marker (a plain non-plugin) — to prove the preflight rejects it.
func writeNonPluginDist(t *testing.T) string {
	t.Helper()
	dist := writeMinimalDist(t)
	// Overwrite the binary with non-plugin bytes.
	if err := os.WriteFile(filepath.Join(dist, "p_linux_amd64_v1", "hello"), []byte("just a hello-world, no handshake"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dist
}

// jwtWithAccess builds a JWT whose payload carries the granted access actions.
func jwtWithAccess(repo string, actions []string) string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]any{
		"access": []map[string]any{{"type": "repository", "name": repo, "actions": actions}},
	})
	return fmt.Sprintf("%s.%s.sig", hdr, base64.RawURLEncoding.EncodeToString(payload))
}

// mockHub serves discovery + /v2/token. tokenActions are what /v2/token grants.
// pushHit is flipped true if zot's blob-upload (push) is ever called — the test
// asserts it stays false on the ownership-denied path.
func mockHub(t *testing.T, tokenActions []string, pushHit *bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/grc-store-configuration", func(w http.ResponseWriter, _ *http.Request) {
		// registry_url points back at this same server (it also fields /v2/* so a
		// stray push would be observable), api_version + oidc fields present.
		_, _ = fmt.Fprintf(w, `{"registry_url":%q,"hub_url":%q,"api_version":"v1","oidc_issuer":"https://issuer","oidc_cli_client_id":"grcli"}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/v2/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"token":%q}`, jwtWithAccess("acme/plugins/hello", tokenActions))
	})
	// Any /v2/<repo>/blobs/uploads/ means a push was attempted — must NOT happen
	// on the ownership-denied path.
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/uploads/") || r.Method == http.MethodPut {
			if pushHit != nil {
				*pushHit = true
			}
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv = httptest.NewServer(mux)
	return srv
}
