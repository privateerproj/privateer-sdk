package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// tokenEnv is an explicit bearer override (CI trusted-publishing or a manually
// minted token), checked before the device-grant store. Mirrors grcli's
// GRCLI_TOKEN escape hatch.
const tokenEnv = "PVTR_TOKEN"

// Login runs the device-authorization grant against the issuer and stores the
// resulting credentials. promptOut receives the user-facing "open this URL,
// enter this code" message. It returns the canonical issuer it logged into.
func Login(ctx context.Context, issuer, clientID string, promptOut io.Writer) (string, error) {
	if clientID == "" {
		return "", errors.New("the hub discovery doc did not advertise oidc_cli_client_id; cannot run device login")
	}
	meta, err := FetchOIDCMetadata(ctx, issuer)
	if err != nil {
		return "", err
	}
	da, err := StartDeviceFlow(ctx, meta, clientID)
	if err != nil {
		return "", err
	}

	target := da.VerificationURIComplete
	if target == "" {
		target = da.VerificationURI
	}
	_, _ = fmt.Fprintf(promptOut, "To authorize pvtr, open:\n  %s\nand enter code: %s\n\nWaiting for authorization...\n", target, da.UserCode)

	creds, err := PollForToken(ctx, meta, clientID, da)
	if err != nil {
		return "", err
	}
	store, err := NewDefaultStore()
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
	store, err := NewDefaultStore()
	if err != nil {
		return err
	}
	return store.Delete(issuer)
}

// BearerToken resolves an OIDC bearer to authenticate registry/hub writes.
// Resolution order (highest first):
//  1. PVTR_TOKEN env — an explicit token (CI trusted-publishing's GHA-OIDC
//     token, or a manually minted one). No store interaction.
//  2. The device-grant store for the given issuer, refreshing if near expiry.
//
// Returns ErrNoCredentials when neither is available — callers map that to
// "run `pvtr login` (or set PVTR_TOKEN in CI)".
func BearerToken(ctx context.Context, issuer, clientID string) (string, error) {
	if tok := strings.TrimSpace(os.Getenv(tokenEnv)); tok != "" {
		return tok, nil
	}
	store, err := NewDefaultStore()
	if err != nil {
		return "", err
	}
	creds, err := store.Get(issuer)
	if err != nil {
		return "", err // ErrNoCredentials propagates
	}
	if !creds.Expired() {
		return creds.AccessToken, nil
	}
	// Near/at expiry: refresh if we can, else require a fresh login.
	if creds.RefreshToken == "" {
		return "", fmt.Errorf("stored credentials for %s expired and carry no refresh token; run `pvtr login`", issuer)
	}
	meta, err := FetchOIDCMetadata(ctx, issuer)
	if err != nil {
		return "", err
	}
	if clientID == "" {
		return "", errors.New("cannot refresh: the hub discovery doc did not advertise oidc_cli_client_id")
	}
	refreshed, err := refreshToken(ctx, meta, clientID, creds.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refreshing expired credentials (run `pvtr login` if this keeps failing): %w", err)
	}
	if perr := store.Put(refreshed); perr != nil {
		// A failed cache write must not fail the operation — we still hold a valid
		// token — but it must be VISIBLE: under refresh-token rotation the on-disk
		// token is now consumed, so the next run will force a re-login.
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to cache refreshed credentials (next run may require `pvtr login`): %v\n", perr)
	}
	return refreshed.AccessToken, nil
}
