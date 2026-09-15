package catalog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/oci"
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
	if err := Install(context.Background(), &w, nil, dir, []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "cached" {
		t.Fatalf("cached file was rewritten: %q", got)
	}
}

// TestFetch_LiveHub pulls and verifies the published OSPS Baseline from the
// production hub. It needs the network, so it only runs with PVTR_LIVE_HUB=1.
func TestFetch_LiveHub(t *testing.T) {
	if os.Getenv("PVTR_LIVE_HUB") == "" {
		t.Skip("set PVTR_LIVE_HUB=1 to run against hub.grc.store")
	}
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v2026.08.26"}
	vc, err := Fetch(context.Background(), oci.NewClient(), c)
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
