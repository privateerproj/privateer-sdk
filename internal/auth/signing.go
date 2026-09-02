package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	clientauth "github.com/gemaraproj/grc-store-clientkit/auth"
	"github.com/sigstore/sigstore/pkg/oauthflow"
)

// sigstoreOIDCIssuer is the public OIDC issuer (Dex) that public-good Fulcio
// trusts for interactive signing. The grc.store registry/hub Keycloak is NOT a
// public-good-Fulcio issuer, so its token cannot sign — this is a separate auth.
const sigstoreOIDCIssuer = "https://oauth2.sigstore.dev/auth"

// sigstoreClientID is the public client id for the interactive sigstore flow
// (the same cosign uses).
const sigstoreClientID = "sigstore"

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

	// 2. GitHub Actions ambient OIDC, requested with Fulcio's audience rather
	// than the hub's.
	if clientauth.InGitHubActions() {
		return clientauth.FetchGitHubActionsToken(ctx, fulcioAudience)
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
