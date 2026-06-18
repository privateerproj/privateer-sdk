package oci

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/revanite-io/grc-store-protocol/syncapi"
)

func TestSync_RequestShapeAndBearer(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody syncapi.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Sync(context.Background(), srv.URL, "acme/hello", "0.1.0", "bearer-tok"); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if gotPath != "/v1/plugins/acme/hello/sync" {
		t.Errorf("path = %q", gotPath)
	}
	// Body repository must be the plugins-shape, tag the version.
	if gotBody.Repository != "acme/plugins/hello" {
		t.Errorf("repository = %q", gotBody.Repository)
	}
	if gotBody.Tag != "0.1.0" {
		t.Errorf("tag = %q", gotBody.Tag)
	}
	if gotAuth != "Bearer bearer-tok" {
		t.Errorf("authorization = %q", gotAuth)
	}
}

func TestSync_SurfacesHubError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"plugin_unsigned"}`))
	}))
	defer srv.Close()
	err := Sync(context.Background(), srv.URL, "acme/hello", "0.1.0", "bearer")
	if err == nil {
		t.Fatal("expected error on 422")
	}
	// The hub's actionable code is surfaced verbatim.
	if want := "plugin_unsigned"; !contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
