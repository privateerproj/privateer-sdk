// Package auth is pvtr's thin layer over grc-store-clientkit's OIDC machinery:
// the device-grant login, the credential store, and the token resolution that
// authenticate `pvtr publish` against grc.store (ADR-0028).
//
// The flows themselves are no longer implemented here. They were, and so were
// near-identical copies of them in grcli — 148 identical lines of credential
// store and 187 of device grant — until both moved to
// github.com/gemaraproj/grc-store-clientkit. What remains in this package is
// what is genuinely pvtr's: its prompt wording, its App identity, and the
// Sigstore signing identity in signing.go, which is a DIFFERENT token from a
// DIFFERENT issuer and deliberately not shared.
//
// The consumer (install) path stays anonymous and does not touch this package.
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
// file at ${XDG_DATA_HOME:-~/.local/share}/pvtr/credentials.json — the same
// path pvtr has always written, and deliberately NOT grcli's, so the two tools
// cannot clobber each other's tokens — and names pvtr in every "run `pvtr
// login`" hint the shared code emits.
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
		// The shared sentinels carry no tool name — one package serves both
		// pvtr and grcli — so the "what do I do now" half is added here.
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
// login`, so the fix is in the message rather than in the reader's head.
//
// This is NOT a signing identity. Fulcio trusts public OIDC issuers, not the
// grc.store Keycloak — see SigningIDToken.
func BearerToken(ctx context.Context, issuer, clientID string) (string, error) {
	in := clientauth.ResolveInput{
		App:      pvtrApp,
		Issuer:   issuer,
		ClientID: clientID,
		Warn:     os.Stderr,
	}
	// A store that cannot be located must not mask PVTR_TOKEN, which Resolve
	// consults first and which is the whole CI path — so this is best-effort.
	if store, err := clientauth.NewDefaultStore(pvtrApp); err == nil {
		in.Store = store
	}
	return clientauth.Resolve(ctx, in)
}
