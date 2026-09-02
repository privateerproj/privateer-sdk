package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	clientauth "github.com/gemaraproj/grc-store-clientkit/auth"
)

// storeUnder builds the store pvtrApp resolves to under an XDG root, so a test
// can seed credentials the wrappers will find. It asserts the path so that a
// change in where grc-store-clientkit places an App's file fails here.
func storeUnder(t *testing.T, xdgRoot string) *clientauth.Store {
	t.Helper()
	s, err := clientauth.NewDefaultStore(pvtrApp)
	if err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}
	if want := filepath.Join(xdgRoot, "pvtr", "credentials.json"); s.Path != want {
		t.Fatalf("credential store at %s, want %s", s.Path, want)
	}
	return s
}

// BearerToken resolution order: PVTR_TOKEN wins over the store; with neither,
// the error names both sources and points at `pvtr login`.
func TestBearerToken_ResolutionOrder(t *testing.T) {
	// Point the store at an empty temp dir so a real ~/.local store can't
	// interfere with either case.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// 1. PVTR_TOKEN set → returned verbatim, no store touched.
	t.Setenv(pvtrApp.TokenEnv, "ci-oidc-token")
	tok, err := BearerToken(context.Background(), "https://issuer", "pvtr-cli")
	if err != nil {
		t.Fatalf("with PVTR_TOKEN set: %v", err)
	}
	if tok != "ci-oidc-token" {
		t.Errorf("PVTR_TOKEN should win, got %q", tok)
	}

	// 2. Neither → an actionable error, not a bare "not found".
	t.Setenv(pvtrApp.TokenEnv, "")
	_, err = BearerToken(context.Background(), "https://issuer", "pvtr-cli")
	var noTok *clientauth.ErrNoToken
	if !errors.As(err, &noTok) {
		t.Fatalf("expected *ErrNoToken, got %v", err)
	}
	// The hint must name pvtr, not grcli, and must not offer a --token flag:
	// the shared message names one, but pvtr registers no such flag.
	msg := err.Error()
	if !strings.Contains(msg, "pvtr login") || !strings.Contains(msg, "PVTR_TOKEN") {
		t.Errorf("error should name pvtr and PVTR_TOKEN, got: %v", err)
	}
	if strings.Contains(msg, "--token") {
		t.Errorf("error offers a --token flag pvtr does not have, got: %v", err)
	}
}

// With no issuer to key credentials on, the error must say so rather than blame
// a missing hub URL, which pvtr always has.
func TestBearerToken_NoIssuerNamesTheRealCause(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv(pvtrApp.TokenEnv, "")

	_, err := BearerToken(context.Background(), "", "pvtr-cli")
	if err == nil {
		t.Fatal("expected an error with no token and no issuer")
	}
	if msg := err.Error(); !strings.Contains(msg, "no OIDC issuer") || strings.Contains(msg, "hub URL") {
		t.Errorf("error should blame the missing issuer, not a missing hub URL, got: %v", err)
	}
}

// Login appends pvtr's own hint to the shared expired-device-code sentinel.
// That wrapping is the only pvtr-specific branch left in the login path.
func TestLogin_ExpiredDeviceCodeNamesPvtr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"device_authorization_endpoint":"%s/device","token_endpoint":"%s/token"}`,
				"http://"+r.Host, "http://"+r.Host, "http://"+r.Host)
		case "/device":
			// interval 1 is the floor: clientkit clamps 0 to RFC 8628's 5s default.
			_, _ = fmt.Fprint(w, `{"device_code":"dc","user_code":"UC","verification_uri":"https://example.test/device","expires_in":60,"interval":1}`)
		case "/token":
			_, _ = fmt.Fprint(w, `{"error":"expired_token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := Login(context.Background(), srv.URL, "pvtr-cli", io.Discard)
	if !errors.Is(err, clientauth.ErrExpiredDeviceCode) {
		t.Fatalf("expected ErrExpiredDeviceCode, got %v", err)
	}
	if !strings.Contains(err.Error(), "pvtr login") {
		t.Errorf("error should point at `pvtr login`, got: %v", err)
	}
}

// A valid (non-expired) stored credential is returned without any network call.
func TestBearerToken_FromStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv(pvtrApp.TokenEnv, "")

	s := storeUnder(t, dir)
	if err := s.Put(&clientauth.Credentials{
		Issuer:      "https://issuer",
		AccessToken: "stored-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	tok, err := BearerToken(context.Background(), "https://issuer", "pvtr-cli")
	if err != nil {
		t.Fatalf("BearerToken: %v", err)
	}
	if tok != "stored-token" {
		t.Errorf("got %q, want stored-token", tok)
	}
}

// When the store's Put fails after a successful refresh (an unwritable
// directory), BearerToken must still return the refreshed access token: under
// refresh-token rotation the old token is already consumed.
func TestBearerToken_StoreWriteFailureReturnsToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based read-only dir test not applicable on Windows")
	}

	// A minimal OIDC server: discovery plus a token endpoint that honors any
	// refresh grant.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"device_authorization_endpoint":"%s/device","token_endpoint":"%s/token"}`,
				"http://"+r.Host, "http://"+r.Host, "http://"+r.Host)
		case "/token":
			_, _ = fmt.Fprint(w, `{"access_token":"refreshed-token","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv(pvtrApp.TokenEnv, "")

	s := storeUnder(t, dir)
	if err := s.Put(&clientauth.Credentials{
		Issuer:       srv.URL,
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour), // already expired
	}); err != nil {
		t.Fatal(err)
	}

	// Read-only store directory: os.CreateTemp cannot land the refreshed file.
	storeDir := filepath.Dir(s.Path)
	if err := os.Chmod(storeDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(storeDir, 0o700) }) // so TempDir cleanup works

	tok, err := BearerToken(context.Background(), srv.URL, "client-id")
	if err != nil {
		t.Fatalf("BearerToken returned an error despite a valid refreshed token: %v", err)
	}
	if tok != "refreshed-token" {
		t.Errorf("got token %q, want refreshed-token", tok)
	}
}
