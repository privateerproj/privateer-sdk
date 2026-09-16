package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
)

func TestInstall_CachedVersionIsNotRefetched(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	path := pluginkit.CatalogCachePath(dir, c)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A nil hub client would panic on any fetch, so success proves the cache
	// short-circuited before touching the network.
	var w bytes.Buffer
	if err := Install(context.Background(), &w, nil, dir, []pluginkit.CatalogCoordinate{c}, true); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "cached" {
		t.Fatalf("cached file was rewritten: %q", got)
	}
}

// mockCatalogHub serves discovery plus a catalog-detail endpoint that 404s, so
// a fetch reaches ErrCatalogNotFound without a registry or a signature. Follows
// the shape of mockInstallHub in internal/install/resolve_test.go.
func mockCatalogHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/grc-store-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"registry_url":%q,"hub_url":%q,"api_version":"v1"}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/v1/catalogs/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A catalog the hub does not know is skipped with a warning for `pvtr install`
// (an older plugin's evaluates can name a catalog it embeds) and fatal for
// `pvtr install --local` (those coordinates come only from AddCatalogs).
func TestInstall_NotFoundSkipsOrFailsByCaller(t *testing.T) {
	hub := mockCatalogHub(t)
	t.Setenv("PVTR_HUB_URL", hub.URL)
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}

	var skipped bytes.Buffer
	if err := Install(context.Background(), &skipped, oci.NewClient(), t.TempDir(), []pluginkit.CatalogCoordinate{c}, true); err != nil {
		t.Fatalf("skipUnpublished=true must not fail: %v", err)
	}
	if !strings.Contains(skipped.String(), "not published on grc.store; skipping") {
		t.Errorf("expected a skip warning, got: %q", skipped.String())
	}

	var fatal bytes.Buffer
	err := Install(context.Background(), &fatal, oci.NewClient(), t.TempDir(), []pluginkit.CatalogCoordinate{c}, false)
	if !errors.Is(err, oci.ErrCatalogNotFound) {
		t.Fatalf("skipUnpublished=false must surface ErrCatalogNotFound, got %v", err)
	}
	if strings.Contains(fatal.String(), "skipping") {
		t.Errorf("a fatal install must not also print the skip warning: %q", fatal.String())
	}
}

// A versionless coordinate is rejected before any hub call, so nothing can
// resolve to "whatever is latest today" and write a cache file Mobilize (which
// reads <version>.yaml) would never load.
func TestFetch_RejectsEmptyVersion(t *testing.T) {
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline"}
	// A nil hub would panic if the guard did not return first.
	_, err := Fetch(context.Background(), &bytes.Buffer{}, nil, c)
	if err == nil || !strings.Contains(err.Error(), "has no version") {
		t.Fatalf("got %v, want a no-version error", err)
	}
}

// The cache write is driven through the fetch seam: real verification needs a
// live signature, but everything after it is ordinary file work worth covering.
func TestInstall_WritesVerifiedCatalogToCache(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	calls := 0
	fetch := func(_ context.Context, _ io.Writer, _ *oci.Client, got pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
		calls++
		if got != c {
			t.Errorf("fetched %s, want %s", got, c)
		}
		return &verify.VerifiedCatalog{YAML: []byte("metadata:\n  id: osps-baseline\n"), SignerIdentity: "keyless:example#wf"}, nil
	}

	var w bytes.Buffer
	if err := install(context.Background(), &w, nil, dir, []pluginkit.CatalogCoordinate{c}, false, fetch); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.ReadFile(pluginkit.CatalogCachePath(dir, c))
	if err != nil {
		t.Fatalf("catalog was not cached: %v", err)
	}
	if !strings.Contains(string(got), "osps-baseline") {
		t.Errorf("cached bytes = %q", got)
	}
	if !strings.Contains(w.String(), "Installed catalog openssf/osps-baseline@v1 (signed by keyless:example#wf)") {
		t.Errorf("progress = %q", w.String())
	}

	// Second pass: the version is immutable, so the cached file short-circuits.
	if err := install(context.Background(), &w, nil, dir, []pluginkit.CatalogCoordinate{c}, false, fetch); err != nil {
		t.Fatalf("install (cached): %v", err)
	}
	if calls != 1 {
		t.Errorf("fetched %d times, want 1", calls)
	}
}

// TestFetch_LiveHub pulls and verifies the published OSPS Baseline from the
// production hub. It needs the network, so it only runs with PVTR_LIVE_HUB=1.
// The registry-divergence branch at Fetch has no offline test: reaching it
// needs a fake registry that serves a real manifest. Tracked as a follow-up.
func TestFetch_LiveHub(t *testing.T) {
	if os.Getenv("PVTR_LIVE_HUB") == "" {
		t.Skip("set PVTR_LIVE_HUB=1 to run against hub.grc.store")
	}
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v2026.08.26"}
	vc, err := Fetch(context.Background(), os.Stderr, oci.NewClient(), c)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	cat, err := pluginkit.ParseCatalog(vc.YAML)
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	if cat.Metadata.Id != "osps-baseline" || len(cat.Controls) == 0 {
		t.Fatalf("unexpected catalog: id=%q controls=%d", cat.Metadata.Id, len(cat.Controls))
	}
	t.Logf("verified %s: digest %s, signed by %s, %d controls", c, vc.ManifestDigest, vc.SignerIdentity, len(cat.Controls))
}
