package oci

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const detailBody = `{
  "namespace":"ossf","plugin_id":"pvtr-github-repo",
  "latest_version":"1.4.0",
  "signer_identity":"keyless:https://token.actions.githubusercontent.com#https://github.com/ossf/pvtr-github-repo/.github/workflows/release.yml",
  "releases":[
    {"version":"1.4.0","index_digest":"sha256:aa","signed":true},
    {"version":"1.3.0","index_digest":"sha256:bb","signed":true}
  ]
}`

func TestGetPluginDetail_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/plugins/ossf/pvtr-github-repo" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(detailBody))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	d, err := c.GetPluginDetails(context.Background(), "ossf", "pvtr-github-repo")
	if err != nil {
		t.Fatalf("GetPluginDetails: %v", err)
	}
	if d.Coordinate() != "ossf/pvtr-github-repo" {
		t.Errorf("coordinate = %q", d.Coordinate())
	}
	if d.LatestVersion != "1.4.0" {
		t.Errorf("latest = %q", d.LatestVersion)
	}
	if d.SignerIdentity == "" {
		t.Error("signer_identity should be present on the plugin-level endpoint")
	}
}

func TestGetPluginDetail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.GetPluginDetails(context.Background(), "nope", "nonexistent")
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("expected ErrPluginNotFound, got %v", err)
	}
}

func TestResolveRelease(t *testing.T) {
	d := &PluginDetail{
		Namespace: "ossf", PluginID: "pvtr-github-repo", LatestVersion: "1.4.0",
		Releases: []PluginRelease{
			{Version: "1.4.0", IndexDigest: "sha256:aa", Signed: true},
			{Version: "1.3.0", IndexDigest: "sha256:bb", Signed: true},
		},
	}

	// Empty requestedVersion resolves to the latest release.
	r, err := d.ResolveRelease("")
	if err != nil {
		t.Fatalf("ResolveRelease empty: %v", err)
	}
	if r.Version != "1.4.0" {
		t.Errorf("latest version = %q, want 1.4.0", r.Version)
	}
	if r.IndexDigest != "sha256:aa" {
		t.Errorf("latest digest = %q, want sha256:aa", r.IndexDigest)
	}

	// Pinned version returns the matching release including its digest.
	r, err = d.ResolveRelease("1.3.0")
	if err != nil {
		t.Fatalf("ResolveRelease 1.3.0: %v", err)
	}
	if r.Version != "1.3.0" {
		t.Errorf("pinned version = %q, want 1.3.0", r.Version)
	}
	if r.IndexDigest != "sha256:bb" {
		t.Errorf("pinned digest = %q, want sha256:bb", r.IndexDigest)
	}

	// Unknown version → error.
	if _, err := d.ResolveRelease("9.9.9"); err == nil {
		t.Error("pin to a non-existent version must error")
	}
}

func TestResolveRelease_NoVersionsPublished(t *testing.T) {
	d := &PluginDetail{Namespace: "ossf", PluginID: "p"}
	if _, err := d.ResolveRelease(""); err == nil {
		t.Error("a plugin with no latest_version must error on default-version resolve")
	}
}

func TestResolveRelease_LatestMissingFromList(t *testing.T) {
	// Hub declares a latest_version that isn't in the releases list:
	// ResolveRelease should synthesise a stub rather than error.
	d := &PluginDetail{
		Namespace: "ossf", PluginID: "p", LatestVersion: "2.0.0",
		Releases: []PluginRelease{
			{Version: "1.4.0", IndexDigest: "sha256:aa"},
		},
	}
	r, err := d.ResolveRelease("")
	if err != nil {
		t.Fatalf("expected stub release, got error: %v", err)
	}
	if r.Version != "2.0.0" {
		t.Errorf("stub version = %q, want 2.0.0", r.Version)
	}
	if r.IndexDigest != "" {
		t.Errorf("stub should have empty digest, got %q", r.IndexDigest)
	}
}
