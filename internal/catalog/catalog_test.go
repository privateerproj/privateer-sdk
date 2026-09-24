package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// stubSource serves verified catalogs from a map; a coordinate not in it is
// ErrCatalogNotFound, and err (when set) is returned for every fetch.
type stubSource struct {
	catalogs map[pluginkit.CatalogCoordinate]*verify.VerifiedCatalog
	err      error
	calls    int
}

func (s *stubSource) Fetch(_ context.Context, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if v, ok := s.catalogs[c]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("%w: %s", oci.ErrCatalogNotFound, c)
}

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
	// A nil source would panic on any fetch, so success proves the cache
	// short-circuited first.
	var w bytes.Buffer
	if _, err := Install(context.Background(), &w, nil, dir, []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "cached" {
		t.Fatalf("cached file was rewritten: %q", got)
	}
}

// A catalog the hub does not know is returned to the caller, which decides
// whether that is a warning or a failure; the rest are still cached.
func TestInstall_ReturnsUnpublishedAndCachesTheRest(t *testing.T) {
	dir := t.TempDir()
	missing1 := pluginkit.CatalogCoordinate{Namespace: "acme", ID: "gone", Version: "v1"}
	present := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	missing2 := pluginkit.CatalogCoordinate{Namespace: "acme", ID: "also-gone", Version: "v2"}
	src := &stubSource{catalogs: map[pluginkit.CatalogCoordinate]*verify.VerifiedCatalog{
		present: {YAML: []byte("metadata:\n  id: osps-baseline\n"), SignerIdentity: "keyless:example#wf"},
	}}

	var w bytes.Buffer
	unpublished, err := Install(context.Background(), &w, src, dir, []pluginkit.CatalogCoordinate{missing1, present, missing2})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(unpublished) != 2 {
		t.Fatalf("unpublished = %v, want 2 errors", unpublished)
	}
	for i, want := range []pluginkit.CatalogCoordinate{missing1, missing2} {
		if !errors.Is(unpublished[i], oci.ErrCatalogNotFound) || !strings.Contains(unpublished[i].Error(), want.String()) {
			t.Errorf("unpublished[%d] = %v, want ErrCatalogNotFound naming %s", i, unpublished[i], want)
		}
	}
	if _, err := os.Stat(pluginkit.CatalogCachePath(dir, present)); err != nil {
		t.Errorf("published catalog was not cached: %v", err)
	}
}

// Anything other than not-found is a verification or transport failure and
// aborts the install.
func TestInstall_FetchFailureAborts(t *testing.T) {
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	boom := errors.New("signature did not verify")
	_, err := Install(context.Background(), &bytes.Buffer{}, &stubSource{err: boom}, t.TempDir(), []pluginkit.CatalogCoordinate{c})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
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

// The hub's 404 surfaces as ErrCatalogNotFound, and the verifier is built only
// when there is something to verify, so a lookup that ends at the hub never
// parses the trust root.
func TestFetcher_NotFoundBeforeVerifier(t *testing.T) {
	hub := mockCatalogHub(t)
	t.Setenv("PVTR_HUB_URL", hub.URL)
	f := NewFetcher(&bytes.Buffer{}, oci.NewClient(), nil)
	_, err := f.Fetch(context.Background(), pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"})
	if !errors.Is(err, oci.ErrCatalogNotFound) {
		t.Fatalf("got %v, want ErrCatalogNotFound", err)
	}
	if f.verifier != nil {
		t.Error("verifier was built for a catalog the hub does not have")
	}
}

// A versionless coordinate is rejected before any hub call, so nothing can
// resolve to "whatever is latest today" and write a cache file Mobilize (which
// reads <version>.yaml) would never load.
func TestFetch_RejectsEmptyVersion(t *testing.T) {
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline"}
	// A nil hub would panic if the guard did not return first.
	_, err := NewFetcher(&bytes.Buffer{}, nil, nil).Fetch(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "has no version") {
		t.Fatalf("got %v, want a no-version error", err)
	}
}

func TestCheckHubDigest(t *testing.T) {
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	var w bytes.Buffer
	if err := checkHubDigest(&w, c, "sha256:aa", "sha256:aa"); err != nil || w.Len() != 0 {
		t.Fatalf("matching digests: err=%v out=%q", err, w.String())
	}
	if err := checkHubDigest(&w, c, "sha256:aa", "sha256:bb"); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("mismatch: got %v, want a diverged error", err)
	}
	w.Reset()
	if err := checkHubDigest(&w, c, "", "sha256:bb"); err != nil || !strings.Contains(w.String(), "no manifest digest") {
		t.Fatalf("missing hub digest: err=%v out=%q, want nil and a warning", err, w.String())
	}
}

// The cache write is driven through a stub source: real verification needs a
// live signature, but everything after it is ordinary file work worth covering.
func TestInstall_WritesVerifiedCatalogToCache(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	src := &stubSource{catalogs: map[pluginkit.CatalogCoordinate]*verify.VerifiedCatalog{
		c: {YAML: []byte("metadata:\n  id: osps-baseline\n"), SignerIdentity: "keyless:example#wf"},
	}}

	var w bytes.Buffer
	if _, err := Install(context.Background(), &w, src, dir, []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("Install: %v", err)
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
	if _, err := Install(context.Background(), &w, src, dir, []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("Install (cached): %v", err)
	}
	if src.calls != 1 {
		t.Errorf("fetched %d times, want 1", src.calls)
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
	vc, err := NewFetcher(os.Stderr, oci.NewClient(), nil).Fetch(context.Background(), c)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	cat := vc.Catalog
	if cat.Metadata.Id != "osps-baseline" || len(cat.Controls) == 0 {
		t.Fatalf("unexpected catalog: id=%q controls=%d", cat.Metadata.Id, len(cat.Controls))
	}
	t.Logf("verified %s: signed by %s, %d controls", c, vc.SignerIdentity, len(cat.Controls))
}
