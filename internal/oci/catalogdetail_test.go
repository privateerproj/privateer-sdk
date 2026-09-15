package oci

import (
	"errors"
	"testing"
)

func TestCatalogDetail_Release(t *testing.T) {
	d := &CatalogDetail{Namespace: "openssf", CatalogID: "osps-baseline", LatestVersion: "v2", Releases: []CatalogRelease{
		{Version: "v1", ManifestDigest: "sha256:1"}, {Version: "v2", ManifestDigest: "sha256:2"},
	}}
	if r, err := d.Release(""); err != nil || r.Version != "v2" {
		t.Errorf("latest: %v %v", r, err)
	}
	if r, err := d.Release("v1"); err != nil || r.ManifestDigest != "sha256:1" {
		t.Errorf("exact: %v %v", r, err)
	}
	// The tag is matched verbatim: a normalized spelling is not the same release.
	if _, err := d.Release("1"); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("missing version: %v", err)
	}
}
