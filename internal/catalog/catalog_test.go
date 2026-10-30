package catalog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
)

// stubSource serves verified catalogs from a map; a coordinate not in it is a
// bare ErrCatalogNotFound (like the hub's 404, which names no version), and err
// (when set) is returned for every fetch.
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
	return nil, oci.ErrCatalogNotFound
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
	pinBytes, err := os.ReadFile(pluginkit.CatalogPinPath(dir, c))
	if err != nil {
		t.Fatalf("catalog signer pin was not written: %v", err)
	}
	if strings.TrimSpace(string(pinBytes)) != "keyless:example#wf" {
		t.Errorf("pinned identity = %q, want %q", string(pinBytes), "keyless:example#wf")
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

func TestCatalogPin_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	pinPath := pluginkit.CatalogPinPath(dir, c)

	// Only a missing file means "no pin".
	if got, err := readCatalogPin(pinPath); got != "" || err != nil {
		t.Fatalf("missing file: got (%q, %v), want (\"\", nil)", got, err)
	}

	// An empty identity write is a no-op.
	if err := writeCatalogPin(pinPath, ""); err != nil {
		t.Fatalf("writeCatalogPin empty identity: %v", err)
	}
	if got, err := readCatalogPin(pinPath); got != "" || err != nil {
		t.Fatalf("after empty write: got (%q, %v), want (\"\", nil)", got, err)
	}

	id := "keyless:https://token.actions.githubusercontent.com#workflow.yml"
	if err := writeCatalogPin(pinPath, id); err != nil {
		t.Fatalf("writeCatalogPin: %v", err)
	}
	if got, err := readCatalogPin(pinPath); got != id || err != nil {
		t.Fatalf("got (%q, %v), want (%q, nil)", got, err, id)
	}
}

// A pin file that exists but cannot be used fails closed rather than reading
// as "no pin", which would let the hub's identity overwrite it.
func TestReadCatalogPin_UnusableFileIsAnError(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty", ".signer")
	if err := os.MkdirAll(filepath.Dir(empty), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCatalogPin(empty); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("empty pin file: got %v, want an error", err)
	}

	isDir := filepath.Join(dir, "dir", ".signer")
	if err := os.MkdirAll(isDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readCatalogPin(isDir); err == nil {
		t.Error("directory at pin path: got nil, want an error")
	}
}

func TestFetcher_PinnedIdentity(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	f := NewFetcher(&bytes.Buffer{}, nil, nil).WithBinariesDir(dir)
	pinPath := pluginkit.CatalogPinPath(dir, c)

	// First fetch (no local pin): seeds from the hub identity, no warning.
	hubID := "keyless:https://issuer#workflow"
	pin, warn, err := f.pinnedIdentity(c, hubID)
	if err != nil || pin != hubID || warn != "" {
		t.Fatalf("first fetch: got (%q, %q, %v), want (%q, \"\", nil)", pin, warn, err, hubID)
	}

	if err := writeCatalogPin(pinPath, hubID); err != nil {
		t.Fatalf("writeCatalogPin: %v", err)
	}

	// Matching hub identity: local pin enforced, no warning.
	if pin, warn, err = f.pinnedIdentity(c, hubID); err != nil || pin != hubID || warn != "" {
		t.Fatalf("matching: got (%q, %q, %v)", pin, warn, err)
	}

	// Differing hub identity: local pin still wins, and the warning names
	// both identities and the .signer file to delete.
	rogueHubID := "keyless:https://attacker#evil-workflow"
	pin, warn, err = f.pinnedIdentity(c, rogueHubID)
	if err != nil || pin != hubID {
		t.Fatalf("local pin must win: got (%q, %v), want %q", pin, err, hubID)
	}
	for _, want := range []string{hubID, rogueHubID, pinPath} {
		if !strings.Contains(warn, want) {
			t.Errorf("warning %q missing %q", warn, want)
		}
	}
}

// pinHub serves discovery and a catalog-detail document declaring hubID and a
// release whose manifest digest matches stubPull's descriptor.
func pinHub(t *testing.T, hubID string) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/grc-store-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"registry_url":%q,"hub_url":%q,"api_version":"v1"}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/v1/catalogs/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"namespace":"openssf","catalog_id":"osps-baseline","latest_version":"v1","signer_identity":%q,"releases":[{"version":"v1","manifest_digest":%q}]}`,
			hubID, stubDesc.Digest.String())
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("PVTR_HUB_URL", srv.URL)
}

var stubDesc = ocispec.Descriptor{
	MediaType: ocispec.MediaTypeImageManifest,
	Digest:    digest.FromString("catalog-manifest"),
}

// stubFetcher returns a Fetcher with the registry pull and sigstore check
// replaced: the catalog is "signed" by signer, and verification fails with
// ErrIdentityMismatch when a pin is enforced and differs, like the real one.
func stubFetcher(dir, signer string) *Fetcher {
	f := NewFetcher(&bytes.Buffer{}, oci.NewClient(), nil).WithBinariesDir(dir)
	f.pull = func(_ context.Context, repository, tag string, _ oci.PullOptions) (*oci.FetchedIndex, error) {
		return oci.NewFetchedIndex(repository, tag, stubDesc, nil, nil, nil), nil
	}
	f.verifyCatalog = func(_ context.Context, _ *oci.FetchedIndex, policy verify.IdentityPolicy) (*verify.VerifiedCatalog, error) {
		if policy.PinnedIdentity != "" && policy.PinnedIdentity != signer {
			return nil, fmt.Errorf("%w: got %s, pinned %s", verify.ErrIdentityMismatch, signer, policy.PinnedIdentity)
		}
		return &verify.VerifiedCatalog{SignerIdentity: signer, YAML: []byte("metadata:\n  id: osps-baseline\n")}, nil
	}
	return f
}

// The mismatch branch of Fetch, driven end to end with stubs: a local pin that
// differs from the catalog's signer fails closed, names the .signer file, and
// leaves the pin untouched.
func TestFetch_LocalPinMismatchFailsClosed(t *testing.T) {
	pinned := "keyless:https://issuer#original"
	pinHub(t, pinned)
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	pinPath := pluginkit.CatalogPinPath(dir, c)
	if err := writeCatalogPin(pinPath, pinned); err != nil {
		t.Fatal(err)
	}

	_, err := stubFetcher(dir, "keyless:https://attacker#evil").Fetch(context.Background(), c)
	if !errors.Is(err, verify.ErrIdentityMismatch) {
		t.Fatalf("got %v, want ErrIdentityMismatch", err)
	}
	if !strings.Contains(err.Error(), pinPath) {
		t.Errorf("error %q does not name the pin file %s", err, pinPath)
	}
	if got, _ := readCatalogPin(pinPath); got != pinned {
		t.Errorf("pin changed to %q, want %q", got, pinned)
	}
}

// Fetch enforces but never writes a pin, so `pvtr publish`, which reads
// catalogs through Fetch, leaves no pin files behind.
func TestFetch_DoesNotWritePin(t *testing.T) {
	signer := "keyless:https://issuer#wf"
	pinHub(t, signer)
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}

	if _, err := stubFetcher(dir, signer).Fetch(context.Background(), c); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := os.Stat(pluginkit.CatalogPinPath(dir, c)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Fetch wrote a pin file (stat err %v)", err)
	}
}

// Install is where the pin is written, through the real Fetcher pin path, and
// a later signer change is then refused.
func TestInstall_PinsThenRefusesNewSigner(t *testing.T) {
	signer := "keyless:https://issuer#wf"
	pinHub(t, "")
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}

	if _, err := Install(context.Background(), &bytes.Buffer{}, stubFetcher(dir, signer), dir, []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, _ := readCatalogPin(pluginkit.CatalogPinPath(dir, c)); got != signer {
		t.Fatalf("pin = %q, want %q", got, signer)
	}

	// Drop the cached YAML (the pin stays) and reinstall from a different
	// signer: the pin refuses it.
	if err := os.Remove(pluginkit.CatalogCachePath(dir, c)); err != nil {
		t.Fatal(err)
	}
	_, err := Install(context.Background(), &bytes.Buffer{}, stubFetcher(dir, "keyless:https://attacker#evil"), dir, []pluginkit.CatalogCoordinate{c})
	if !errors.Is(err, verify.ErrIdentityMismatch) {
		t.Fatalf("got %v, want ErrIdentityMismatch", err)
	}
}

// An unreadable pin aborts Install instead of being overwritten.
func TestInstall_UnreadablePinAborts(t *testing.T) {
	dir := t.TempDir()
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	if err := os.MkdirAll(pluginkit.CatalogPinPath(dir, c), 0o755); err != nil {
		t.Fatal(err)
	}
	src := &stubSource{catalogs: map[pluginkit.CatalogCoordinate]*verify.VerifiedCatalog{
		c: {YAML: []byte("x"), SignerIdentity: "keyless:example#wf"},
	}}
	if _, err := Install(context.Background(), &bytes.Buffer{}, src, dir, []pluginkit.CatalogCoordinate{c}); err == nil {
		t.Fatal("Install succeeded with a directory at the pin path")
	}
}
