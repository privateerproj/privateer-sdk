package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeRegistryJWT builds an unsigned-shape JWT (header.payload.signature) whose
// payload carries the Docker-style `access` claim the hub mints, so the test
// exercises the real grantedActions decode. The signature segment is a
// placeholder — pvtr reads (does not verify) its own granted scope.
func fakeRegistryJWT(t *testing.T, repo string, actions []string) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"access": []map[string]any{
			{"type": "repository", "name": repo, "actions": actions},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return fmt.Sprintf("%s.%s.%s", hdr, body, "sig")
}

func tokenServer(t *testing.T, repo string, actions []string, capture *string, captureAuth *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/token" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if capture != nil {
			*capture = r.URL.Query().Get("scope")
		}
		if captureAuth != nil {
			*captureAuth = r.Header.Get("Authorization")
		}
		_, _ = fmt.Fprintf(w, `{"token":%q}`, fakeRegistryJWT(t, repo, actions))
	}))
}

func TestMintRegistryToken_OwnerGetsPush(t *testing.T) {
	var gotScope, gotAuth string
	srv := tokenServer(t, "acme/plugins/hello", []string{"pull", "push"}, &gotScope, &gotAuth)
	defer srv.Close()

	rt, err := MintRegistryToken(context.Background(), srv.URL, "acme/hello", "upstream-bearer")
	if err != nil {
		t.Fatalf("MintRegistryToken: %v", err)
	}
	if !rt.GrantsPush() {
		t.Errorf("owner token must grant push; actions = %v", rt.Actions)
	}
	if rt.Token == "" {
		t.Error("token is empty")
	}
	if gotScope != "repository:acme/plugins/hello:pull,push" {
		t.Errorf("scope = %q", gotScope)
	}
	if gotAuth != "Bearer upstream-bearer" {
		t.Errorf("authorization = %q", gotAuth)
	}
}

// The hub grants PULL-ONLY (not an error) when the caller doesn't own the
// namespace. MintRegistryToken must succeed but report no push.
func TestMintRegistryToken_NonOwnerGetsPullOnly(t *testing.T) {
	srv := tokenServer(t, "acme/plugins/hello", []string{"pull"}, nil, nil)
	defer srv.Close()

	rt, err := MintRegistryToken(context.Background(), srv.URL, "acme/hello", "upstream-bearer")
	if err != nil {
		t.Fatalf("MintRegistryToken (pull-only is not an error): %v", err)
	}
	if rt.GrantsPush() {
		t.Errorf("pull-only token must NOT grant push; actions = %v", rt.Actions)
	}
}

func TestMintRegistryToken_FailsClosedOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := MintRegistryToken(context.Background(), srv.URL, "acme/hello", "bearer"); err == nil {
		t.Error("expected error on 403")
	}
}

func TestMintRegistryToken_AnonymousWhenNoBearer(t *testing.T) {
	var gotAuth string
	srv := tokenServer(t, "acme/plugins/hello", []string{"pull"}, nil, &gotAuth)
	defer srv.Close()
	if _, err := MintRegistryToken(context.Background(), srv.URL, "acme/hello", ""); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Errorf("anonymous mint should send no Authorization header, got %q", gotAuth)
	}
}

func TestRegistryToken_GrantsPush(t *testing.T) {
	if !(RegistryToken{Actions: []string{"pull", "push"}}).GrantsPush() {
		t.Error("pull,push must grant push")
	}
	if (RegistryToken{Actions: []string{"pull"}}).GrantsPush() {
		t.Error("pull-only must not grant push")
	}
	if (RegistryToken{Actions: nil}).GrantsPush() {
		t.Error("no actions must not grant push")
	}
}
