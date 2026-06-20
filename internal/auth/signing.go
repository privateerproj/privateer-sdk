package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/sigstore/sigstore/pkg/oauthflow"
)

// sigstoreOIDCIssuer is the public OIDC issuer (Dex) that public-good Fulcio
// trusts for interactive signing. The grc.store registry/hub Keycloak is NOT a
// public-good-Fulcio issuer, so its token cannot sign — this is a separate auth.
const sigstoreOIDCIssuer = "https://oauth2.sigstore.dev/auth"

// sigstoreClientID is the public client id for the interactive sigstore flow
// (the same cosign uses).
const sigstoreClientID = "sigstore"

// ghaTokenTimeout bounds the GitHub Actions OIDC token request. It is more
// generous than the hub's httpTimeout because the GHA token service can be
// slower than the hub's own OIDC discovery.
const ghaTokenTimeout = 15 * time.Second

// signingTokenEnv is an explicit OIDC signing-token override: any environment
// that can mint an aud=sigstore token (GitLab CI `id_tokens`, a manually minted
// token, ...) sets it. Checked before ambient detection, mirroring how
// PVTR_TOKEN overrides the bearer in BearerToken.
const signingTokenEnv = "SIGSTORE_ID_TOKEN"

// fulcioAudience is the OIDC audience public-good Fulcio requires on the signing
// identity token.
const fulcioAudience = "sigstore"

// SigningIDToken returns an OIDC ID token suitable for PUBLIC-GOOD Fulcio — the
// identity the keyless plugin signature is minted under. This is DISTINCT from
// BearerToken (the grc.store registry/hub login): Fulcio only trusts public OIDC
// issuers (GitHub Actions, GitLab, Google, the interactive sigstore Dex), not the
// grc.store Keycloak.
//
// Resolution order (highest first):
//  1. SIGSTORE_ID_TOKEN env — an explicit OIDC token with audience "sigstore".
//     The headless escape hatch for any CI that can mint one (e.g. GitLab CI's
//     `id_tokens`), checked first so an explicit override wins over ambient
//     detection — mirroring how PVTR_TOKEN overrides the bearer.
//  2. GitHub Actions ambient OIDC — mints an aud=sigstore token from the runner's
//     token service (a SEPARATE request from the hub bearer).
//  3. Interactive sigstore browser sign-in — a SECOND browser auth on top of
//     `pvtr login`, inherent to keyless signing. Skipped with an actionable error
//     when stdin is not a TTY, so CI fails fast instead of hanging on a flow no
//     one can complete.
//
// promptOut receives any interactive instructions.
func SigningIDToken(ctx context.Context, promptOut io.Writer) (string, error) {
	// 1. Explicit env override.
	if raw := strings.TrimSpace(os.Getenv(signingTokenEnv)); raw != "" {
		if err := validateSigningToken(raw); err != nil {
			return "", err
		}
		return raw, nil
	}

	// 2. GitHub Actions ambient OIDC.
	if tok, ok, err := githubActionsSigningToken(ctx); err != nil {
		return "", err
	} else if ok {
		return tok, nil
	}

	// 3. Interactive — only viable with a human at a terminal.
	//
	// TODO: other Fulcio-trusted CI providers that do NOT expose their OIDC token
	// as a plain env var still need dedicated ambient detectors here — Buildkite
	// (`buildkite-agent oidc request-token --audience sigstore`) and GCP (the
	// metadata server). Until then they use SIGSTORE_ID_TOKEN above. (CircleCI
	// cannot sign against public-good Fulcio at all: its OIDC audience is locked
	// and cannot be set to "sigstore".)
	if !stdinIsTerminal() {
		return "", fmt.Errorf(
			"no Sigstore signing identity available and stdin is not a terminal: set %s to an "+
				"OIDC token with audience %q (e.g. GitLab CI id_tokens), or run in GitHub Actions "+
				"where it is detected automatically", signingTokenEnv, fulcioAudience)
	}
	return interactiveSigningToken(promptOut)
}

// githubActionsSigningToken requests a GHA OIDC ID token with audience
// "sigstore" when running in GitHub Actions (ACTIONS_ID_TOKEN_REQUEST_URL +
// _TOKEN are present). Returns ok=false when not in that environment.
func githubActionsSigningToken(ctx context.Context) (string, bool, error) {
	reqURL := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"))
	reqTok := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"))
	if reqURL == "" || reqTok == "" {
		return "", false, nil
	}
	u, err := url.Parse(reqURL)
	if err != nil {
		return "", false, fmt.Errorf("parsing ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", fulcioAudience)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("building GHA OIDC request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+reqTok)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: ghaTokenTimeout}).Do(req)
	if err != nil {
		return "", false, fmt.Errorf("requesting GHA OIDC token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("GHA OIDC token request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", false, fmt.Errorf("decoding GHA OIDC token: %w", err)
	}
	if out.Value == "" {
		return "", false, fmt.Errorf("GHA OIDC token response had no value")
	}
	return out.Value, true, nil
}

// validateSigningToken does a lightweight, signature-less sanity check of a
// caller-supplied SIGSTORE_ID_TOKEN so a misconfiguration surfaces here with an
// actionable message instead of as an opaque Fulcio rejection later. It only
// inspects the audience claim; Fulcio remains the authority on issuer trust and
// signature validity.
func validateSigningToken(raw string) error {
	auds, err := jwtAudiences(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid JWT: %w", signingTokenEnv, err)
	}
	if slices.Contains(auds, fulcioAudience) {
		return nil
	}
	return fmt.Errorf("%s has audience %q, but public-good Fulcio requires %q — mint the OIDC token with aud=%q",
		signingTokenEnv, auds, fulcioAudience, fulcioAudience)
}

// jwtAudiences extracts the "aud" claim from a JWT WITHOUT verifying its
// signature (Fulcio does that). Per RFC 7519 §4.1.3 "aud" is either a string or
// an array of strings; both are returned as a slice. A missing claim yields an
// empty slice, which the caller treats as an audience mismatch.
func jwtAudiences(raw string) ([]string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected 3 dot-separated segments, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding payload segment: %w", err)
	}
	var claims struct {
		Aud json.RawMessage `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing claims: %w", err)
	}
	if len(claims.Aud) == 0 || string(claims.Aud) == "null" {
		return nil, nil // absent or JSON-null aud → caller treats as a mismatch
	}
	var single string
	if err := json.Unmarshal(claims.Aud, &single); err == nil {
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(claims.Aud, &many); err == nil {
		return many, nil
	}
	return nil, fmt.Errorf(`"aud" claim is neither a string nor an array of strings`)
}

// stdinIsTerminal reports whether stdin is an interactive terminal (a character
// device), used to decide whether the interactive signing flow can run. In CI
// stdin is typically a pipe or /dev/null, so this is false and callers fail fast
// rather than launching a browser flow no one can complete. It is a var so tests
// can stub the terminal check.
var stdinIsTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveSigningToken runs the browser-based sigstore OAuth flow (cosign's
// DefaultIDTokenGetter against the public sigstore Dex).
func interactiveSigningToken(promptOut io.Writer) (string, error) {
	if promptOut != nil {
		_, _ = io.WriteString(promptOut, "Signing requires a public-good Sigstore identity (separate from `pvtr login`).\nA browser window will open to sign in...\n")
	}
	tok, err := oauthflow.OIDConnect(sigstoreOIDCIssuer, sigstoreClientID, "", "", oauthflow.DefaultIDTokenGetter)
	if err != nil {
		return "", fmt.Errorf("interactive sigstore sign-in: %w", err)
	}
	if tok == nil || tok.RawString == "" {
		return "", fmt.Errorf("sigstore sign-in returned no token")
	}
	return tok.RawString, nil
}
