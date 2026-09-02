// Package auth authenticates `pvtr publish` against grc.store. The
// device-grant login, credential store, and token resolution come from
// grc-store-clientkit; this package supplies pvtr's App identity and prompt
// wording, plus the Sigstore signing identity in signing.go, which is a
// separate token from a separate issuer.
//
// The consumer (install) path is anonymous and does not use this package.
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	clientauth "github.com/gemaraproj/grc-store-clientkit/auth"
)

// pvtrApp identifies pvtr to grc-store-clientkit. It selects the credential
// file at ${XDG_DATA_HOME:-~/.local/share}/pvtr/credentials.json, kept separate
// from grcli's so the two tools cannot clobber each other's tokens, and names
// pvtr in the "run `pvtr login`" hints the shared code emits.
var pvtrApp = clientauth.App{Name: "pvtr", TokenEnv: "PVTR_TOKEN"}

// Login runs the device-authorization grant against the issuer and stores the
// resulting credentials. promptOut receives the user-facing "open this URL,
// enter this code" message. It returns the canonical issuer it logged into.
func Login(ctx context.Context, issuer, clientID string, promptOut io.Writer) (string, error) {
	if clientID == "" {
		return "", errors.New("the hub discovery doc did not advertise oidc_cli_client_id; cannot run device login")
	}
	meta, err := clientauth.FetchOIDCMetadata(ctx, issuer)
	if err != nil {
		return "", err
	}
	da, err := clientauth.StartDeviceFlow(ctx, meta, clientID)
	if err != nil {
		return "", err
	}

	target := da.VerificationURIComplete
	if target == "" {
		target = da.VerificationURI
	}
	_, _ = fmt.Fprintf(promptOut, "To authorize pvtr, open:\n  %s\nand enter code: %s\n\nWaiting for authorization...\n", target, da.UserCode)

	creds, err := clientauth.PollForToken(ctx, meta, clientID, da)
	if err != nil {
		// The shared sentinel carries no tool name, so the pvtr-specific hint
		// is appended here.
		if errors.Is(err, clientauth.ErrExpiredDeviceCode) {
			return "", fmt.Errorf("%w — %s again", err, pvtrApp.LoginHint())
		}
		return "", err
	}
	store, err := clientauth.NewDefaultStore(pvtrApp)
	if err != nil {
		return "", err
	}
	if err := store.Put(creds); err != nil {
		return "", err
	}
	return creds.Issuer, nil
}

// Logout forgets stored credentials for the issuer.
func Logout(issuer string) error {
	store, err := clientauth.NewDefaultStore(pvtrApp)
	if err != nil {
		return err
	}
	return store.Delete(issuer)
}

// BearerToken resolves an OIDC bearer to authenticate registry/hub writes.
// Resolution order (highest first):
//
//  1. PVTR_TOKEN — an explicit token (CI trusted-publishing's GHA-OIDC token,
//     or a manually minted one). No store interaction.
//  2. The device-grant store for the given issuer, refreshing if near expiry.
//
// When neither is available the error names both sources and points at `pvtr
// login`.
//
// This is not a signing identity; Fulcio trusts public OIDC issuers, not the
// grc.store Keycloak. See SigningIDToken.
func BearerToken(ctx context.Context, issuer, clientID string) (string, error) {
	in := clientauth.ResolveInput{
		App:      pvtrApp,
		Issuer:   issuer,
		ClientID: clientID,
		Warn:     os.Stderr,
	}
	// Store lookup is best-effort: a missing store must not mask PVTR_TOKEN,
	// which Resolve consults first. The error is kept rather than dropped so the
	// no-token path can name the real cause.
	store, storeErr := clientauth.NewDefaultStore(pvtrApp)
	if storeErr == nil {
		in.Store = store
	}
	tok, err := clientauth.Resolve(ctx, in)
	var noTok *clientauth.ErrNoToken
	if errors.As(err, &noTok) {
		return "", &noTokenError{inner: noTok, msg: noTokenMessage(issuer, storeErr)}
	}
	return tok, err
}

// noTokenError restates the shared ErrNoToken in pvtr's terms. The shared
// message opens with "--token unset", naming a flag pvtr does not register;
// Unwrap keeps errors.Is/As matching the sentinel for callers that type-check.
type noTokenError struct {
	inner *clientauth.ErrNoToken
	msg   string
}

func (e *noTokenError) Error() string { return e.msg }
func (e *noTokenError) Unwrap() error { return e.inner }

// noTokenMessage names the source that is actually missing. The shared text
// blames a missing hub URL whenever the store was not consulted, which for pvtr
// is never the reason: pvtr always has a hub, so the cause is either an
// unlocatable store or a hub that advertises no issuer to key credentials on.
func noTokenMessage(issuer string, storeErr error) string {
	switch {
	case storeErr != nil:
		return fmt.Sprintf("no token available: %s unset and the credential store could not be located (%v) — set %s, or fix the data directory and %s",
			pvtrApp.TokenEnv, storeErr, pvtrApp.TokenEnv, pvtrApp.LoginHint())
	case issuer == "":
		return fmt.Sprintf("no token available: %s unset and the hub advertises no OIDC issuer, so there are no stored credentials to consult — set %s",
			pvtrApp.TokenEnv, pvtrApp.TokenEnv)
	default:
		return fmt.Sprintf("no token available: %s unset and no stored credentials for %s — %s",
			pvtrApp.TokenEnv, issuer, pvtrApp.LoginHint())
	}
}
