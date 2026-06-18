package install

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockInstallHub serves discovery + plugin-detail. found controls whether the
// coordinate resolves; pullHit flips true if a registry pull (/v2/) is reached,
// proving resolution succeeded and handed off to the verified pull core.
func mockInstallHub(t *testing.T, found bool, pullHit *bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/ext.grc-store", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"registry_url":%q,"hub_url":%q,"api_version":"v1"}`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/v1/plugins/", func(w http.ResponseWriter, _ *http.Request) {
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"namespace":"acme","plugin_id":"hello","latest_version":"0.1.0",` +
			`"releases":[{"version":"0.1.0","index_digest":"sha256:aa","signed":true}]}`))
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, _ *http.Request) {
		if pullHit != nil {
			*pullHit = true
		}
		// 401 so the pull fails fast after being reached (we only assert reach).
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv = httptest.NewServer(mux)
	return srv
}

// A coordinate the hub doesn't have → a clear "not found", terminal before any
// registry pull.
func TestFromStore_NotFound(t *testing.T) {
	pullHit := false
	hub := mockInstallHub(t, false, &pullHit)
	defer hub.Close()
	t.Setenv("PVTR_HUB_URL", hub.URL)

	err := FromStore(context.Background(), io.Discard, "acme/nonexistent")
	if err == nil {
		t.Fatal("expected an error for a coordinate not on grc.store")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found, got: %v", err)
	}
	if pullHit {
		t.Error("must not attempt a registry pull for a non-existent plugin")
	}
}

// A found coordinate resolves and proceeds to the verified pull core (which then
// fails at the mock registry — we only assert resolution succeeded and pull was
// reached, NOT an unverified fallback).
func TestFromStore_FoundProceedsToPull(t *testing.T) {
	pullHit := false
	hub := mockInstallHub(t, true, &pullHit)
	defer hub.Close()
	t.Setenv("PVTR_HUB_URL", hub.URL)

	err := FromStore(context.Background(), io.Discard, "acme/hello")
	// It must NOT succeed (no real signed index behind the mock), and must NOT
	// be the resolution error — it should fail later, at pull/verify.
	if err == nil {
		t.Fatal("expected the install to fail at pull/verify (no real index), got success")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("a found plugin must get PAST resolution, got: %v", err)
	}
	if !pullHit {
		t.Error("a found plugin should proceed to the registry pull")
	}
}

// A bare name (no namespace) is rejected with the coordinate-form guidance —
// grc.store has no default namespace (the reversed Phase-B default-owner break).
func TestFromStore_BareNameRejected(t *testing.T) {
	err := FromStore(context.Background(), io.Discard, "pvtr-github-repo")
	if err == nil {
		t.Fatal("expected a bare name to be rejected")
	}
	if !strings.Contains(err.Error(), "<namespace>/<plugin_id>") {
		t.Errorf("bare-name error should show the coordinate form, got: %v", err)
	}
}

// @version pin resolves the requested version.
func TestFromStore_VersionPin(t *testing.T) {
	pullHit := false
	hub := mockInstallHub(t, true, &pullHit)
	defer hub.Close()
	t.Setenv("PVTR_HUB_URL", hub.URL)

	// 0.1.0 exists in the mock's releases; pinning it must resolve and pull.
	err := FromStore(context.Background(), io.Discard, "acme/hello@0.1.0")
	if err != nil && strings.Contains(err.Error(), "has no version") {
		t.Fatalf("pin to an existing version should resolve, got: %v", err)
	}
	if !pullHit {
		t.Error("a valid version pin should proceed to pull")
	}

	// A non-existent pin must fail at resolution, before pull.
	pullHit = false
	err = FromStore(context.Background(), io.Discard, "acme/hello@9.9.9")
	if err == nil || !strings.Contains(err.Error(), "no version") {
		t.Errorf("pin to a missing version should fail at resolution, got: %v", err)
	}
	if pullHit {
		t.Error("a bad version pin must not reach pull")
	}
}
