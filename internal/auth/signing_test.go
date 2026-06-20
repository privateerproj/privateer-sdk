package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NOTE: tests in this file swap the package-level stdinIsTerminal var; none may
// call t.Parallel() while doing so (this is a CLI package with no concurrency).

// makeSigningJWT builds a structurally-valid (UNSIGNED) JWT with the given
// claims so the audience-inspection path can be exercised without a real IdP.
func makeSigningJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	b64 := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	header := b64(map[string]string{"alg": "none", "typ": "JWT"})
	payload := b64(claims)
	sig := base64.RawURLEncoding.EncodeToString([]byte("notverified"))
	return header + "." + payload + "." + sig
}

// clearAmbientSigningEnv removes the env that would otherwise route resolution to
// the explicit-token or GitHub-Actions paths, isolating the case under test.
func clearAmbientSigningEnv(t *testing.T) {
	t.Helper()
	t.Setenv(signingTokenEnv, "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
}

func TestSigningIDToken_EnvToken_SigstoreAudience(t *testing.T) {
	clearAmbientSigningEnv(t)
	tok := makeSigningJWT(t, map[string]any{"aud": fulcioAudience, "iss": "https://gitlab.example"})
	t.Setenv(signingTokenEnv, tok)

	got, err := SigningIDToken(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != tok {
		t.Errorf("expected the env token returned verbatim")
	}
}

func TestSigningIDToken_EnvToken_AudienceArray(t *testing.T) {
	clearAmbientSigningEnv(t)
	tok := makeSigningJWT(t, map[string]any{"aud": []string{"other", fulcioAudience}})
	t.Setenv(signingTokenEnv, tok)

	if _, err := SigningIDToken(context.Background(), io.Discard); err != nil {
		t.Errorf("array audience containing %q should be accepted, got: %v", fulcioAudience, err)
	}
}

func TestSigningIDToken_EnvToken_WrongAudienceFails(t *testing.T) {
	clearAmbientSigningEnv(t)
	tok := makeSigningJWT(t, map[string]any{"aud": "some-other-audience"})
	t.Setenv(signingTokenEnv, tok)

	_, err := SigningIDToken(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("expected an error for a non-sigstore audience")
	}
	if !strings.Contains(err.Error(), "audience") || !strings.Contains(err.Error(), fulcioAudience) {
		t.Errorf("error should explain the audience mismatch, got: %v", err)
	}
}

func TestSigningIDToken_EnvToken_NotAJWTFails(t *testing.T) {
	clearAmbientSigningEnv(t)
	t.Setenv(signingTokenEnv, "this-is-not-a-jwt")

	_, err := SigningIDToken(context.Background(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not a valid JWT") {
		t.Errorf("expected a 'not a valid JWT' error, got: %v", err)
	}
}

// The fail-fast guard: no explicit token, not in GitHub Actions, and stdin is not
// a terminal -> a clear error naming SIGSTORE_ID_TOKEN, NOT a hang on the
// interactive browser flow.
func TestSigningIDToken_NonInteractive_FailsFast(t *testing.T) {
	clearAmbientSigningEnv(t)
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = orig })

	_, err := SigningIDToken(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("expected a fail-fast error in a non-interactive environment")
	}
	if !strings.Contains(err.Error(), signingTokenEnv) || !strings.Contains(err.Error(), "not a terminal") {
		t.Errorf("error should name %s and explain the non-terminal condition, got: %v", signingTokenEnv, err)
	}
}

func TestJwtAudiences_MissingClaimIsEmpty(t *testing.T) {
	tok := makeSigningJWT(t, map[string]any{"iss": "https://example"}) // no aud
	auds, err := jwtAudiences(tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auds) != 0 {
		t.Errorf("expected no audiences, got %v", auds)
	}
}

// aud: null is a legal JSON value but not a usable audience; it must be rejected
// cleanly (same path as a missing claim), not pass through as an empty string.
func TestSigningIDToken_EnvToken_NullAudienceRejected(t *testing.T) {
	clearAmbientSigningEnv(t)
	tok := makeSigningJWT(t, map[string]any{"aud": nil})
	t.Setenv(signingTokenEnv, tok)
	if _, err := SigningIDToken(context.Background(), io.Discard); err == nil {
		t.Error("expected null aud to be rejected")
	}
}

// ghaTokenServer stands in for GitHub Actions' OIDC token service.
func ghaTokenServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-token")
}

func TestGithubActionsSigningToken_HappyPath(t *testing.T) {
	t.Setenv(signingTokenEnv, "") // ensure GHA path, not the env override
	jwt := makeSigningJWT(t, map[string]any{"aud": fulcioAudience})
	var gotAudience, gotAuth string
	ghaTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAudience = r.URL.Query().Get("audience")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": jwt})
	})

	tok, ok, err := githubActionsSigningToken(context.Background())
	if err != nil || !ok {
		t.Fatalf("expected a token, got ok=%v err=%v", ok, err)
	}
	if tok != jwt {
		t.Errorf("returned token does not match server response")
	}
	if gotAudience != fulcioAudience {
		t.Errorf("audience query = %q, want %q", gotAudience, fulcioAudience)
	}
	if gotAuth != "Bearer request-token" {
		t.Errorf("authorization header = %q", gotAuth)
	}
}

func TestGithubActionsSigningToken_Non200Errors(t *testing.T) {
	t.Setenv(signingTokenEnv, "")
	ghaTokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no token for you", http.StatusForbidden)
	})
	if _, ok, err := githubActionsSigningToken(context.Background()); err == nil || ok {
		t.Errorf("expected an error on non-200, got ok=%v err=%v", ok, err)
	}
}

func TestGithubActionsSigningToken_EmptyValueErrors(t *testing.T) {
	t.Setenv(signingTokenEnv, "")
	ghaTokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"value": ""})
	})
	if _, ok, err := githubActionsSigningToken(context.Background()); err == nil || ok {
		t.Errorf("expected an error on empty value, got ok=%v err=%v", ok, err)
	}
}

// Not in GitHub Actions (neither env var set) → ok=false, no error.
func TestGithubActionsSigningToken_NotInGHA(t *testing.T) {
	clearAmbientSigningEnv(t)
	tok, ok, err := githubActionsSigningToken(context.Background())
	if err != nil || ok || tok != "" {
		t.Errorf("expected (\"\", false, nil) outside GHA, got (%q, %v, %v)", tok, ok, err)
	}
}
