package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// BearerToken resolution order: PVTR_TOKEN env wins over the store; absent both
// it returns ErrNoCredentials.
func TestBearerToken_ResolutionOrder(t *testing.T) {
	// 1. PVTR_TOKEN set → returned verbatim, no store touched.
	t.Setenv(tokenEnv, "ci-oidc-token")
	// Point the store at an empty temp dir so a stray real ~/.local store can't
	// interfere.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	tok, err := BearerToken(context.Background(), "https://issuer", "grcli")
	if err != nil {
		t.Fatalf("with PVTR_TOKEN set: %v", err)
	}
	if tok != "ci-oidc-token" {
		t.Errorf("PVTR_TOKEN should win, got %q", tok)
	}

	// 2. No PVTR_TOKEN, no stored creds → ErrNoCredentials.
	t.Setenv(tokenEnv, "")
	if _, err := BearerToken(context.Background(), "https://issuer", "grcli"); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
}

// A valid (non-expired) stored credential is returned without any network call.
func TestBearerToken_FromStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv(tokenEnv, "")

	s := &Store{Path: filepath.Join(dir, "pvtr", "credentials.json")}
	if err := s.Put(&Credentials{
		Issuer:      "https://issuer",
		AccessToken: "stored-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	tok, err := BearerToken(context.Background(), "https://issuer", "grcli")
	if err != nil {
		t.Fatalf("BearerToken: %v", err)
	}
	if tok != "stored-token" {
		t.Errorf("got %q, want stored-token", tok)
	}
}

func TestFetchOIDCMetadata_RequiresIssuer(t *testing.T) {
	if _, err := FetchOIDCMetadata(context.Background(), ""); err == nil {
		t.Error("empty issuer must error")
	}
}

// TestBearerToken_StoreWriteFailureReturnsToken verifies that when the
// credential store's Put fails (e.g. the directory is not writable) after a
// successful token refresh, BearerToken still returns the valid access token
// rather than surfacing the write error.  Under refresh-token rotation the
// old token is consumed, so this is a best-effort path — the warning on
// stderr is informational; returning the token is mandatory.
func TestBearerToken_StoreWriteFailureReturnsToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based read-only dir test not applicable on Windows")
	}

	// Spin up a minimal OIDC server: one endpoint serves the discovery doc and
	// another serves the token endpoint (refresh grant).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(OIDCMetadata{
				Issuer:                      "http://" + r.Host,
				DeviceAuthorizationEndpoint: "http://" + r.Host + "/device",
				TokenEndpoint:               "http://" + r.Host + "/token",
			})
		case "/token":
			// Serve a fresh access token for any refresh_token grant.
			_ = json.NewEncoder(w).Encode(tokenResponse{
				AccessToken:  "refreshed-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "new-refresh",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Isolate the store under a temp dir.
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv(tokenEnv, "")

	// Pre-populate the store with expired credentials that carry a refresh
	// token, using the test server as the issuer.
	storeDir := filepath.Join(dir, "pvtr")
	s := &Store{Path: filepath.Join(storeDir, "credentials.json")}
	if err := s.Put(&Credentials{
		Issuer:       srv.URL,
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour), // already expired
	}); err != nil {
		t.Fatal(err)
	}

	// Make the store directory read-only so the Put of the refreshed token
	// will fail (os.CreateTemp can't create a temp file in a non-writable dir).
	if err := os.Chmod(storeDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(storeDir, 0o700) }) // restore so TempDir cleanup works

	tok, err := BearerToken(context.Background(), srv.URL, "client-id")
	if err != nil {
		t.Fatalf("BearerToken returned error despite valid refreshed token: %v", err)
	}
	if tok != "refreshed-token" {
		t.Errorf("got token %q, want %q", tok, "refreshed-token")
	}
}
