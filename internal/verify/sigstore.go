// Package verify implements the §6 consumer verification contract for
// grc.store-sourced plugins: keyless signature verification over a pinned
// public-good Sigstore trusted root, an identity policy (camp (b) TOFU), and
// the full digest-chain walk index → child → config/layer → bytes. Everything
// fails closed — a verification or digest failure ABORTS the install; the path
// never degrades to an unverified copy.
package verify

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/revanite-io/grc-store-protocol/identity"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	sgverify "github.com/sigstore/sigstore-go/pkg/verify"
)

// embeddedTrustedRoot is the pinned public-good Sigstore trusted root
// (Fulcio/Rekor/CTFE). Pinning it — rather than fetching live via TUF — makes
// verification offline and deterministic; the binary self-contains the trust
// material. It is the SAME public-good root the grc.store hub embeds, so a
// GitHub-Actions-keyless signature the hub accepts verifies here identically.
// OPERATIONAL OBLIGATION: refresh this file when the public-good root rotates.
//
//go:embed trusted_root.json
var embeddedTrustedRoot []byte

// Named fail-closed errors. Every one ABORTS the install and is surfaced with
// the signer identity (when known) and coordinate by the caller.
var (
	// ErrUnsigned: the index carries no signature referrer.
	ErrUnsigned = errors.New("plugin index is not signed")
	// ErrSignatureInvalid: a present signature failed cryptographic verification
	// (bad chain, missing SCT, no Rekor inclusion, digest mismatch, etc.).
	ErrSignatureInvalid = errors.New("plugin signature verification failed")
	// ErrIdentityMismatch: the signature is valid but the signer identity does
	// not satisfy the policy (TOFU pin mismatch, or an unexpected SAN).
	ErrIdentityMismatch = errors.New("plugin signer identity does not match the pinned identity")
	// ErrDigestMismatch: a fetched artifact's bytes do not hash to the digest the
	// verified index committed to (any arrow of the walk).
	ErrDigestMismatch = errors.New("plugin digest-chain verification failed")
	// ErrPlatformUnavailable: the verified index has no child for the host
	// os/arch (a first-class checked condition, not a nil-deref).
	ErrPlatformUnavailable = errors.New("no plugin build for this platform")
	// ErrMalformedIndex: the verified index/child/config bytes violate the
	// grc.store shape contract.
	ErrMalformedIndex = errors.New("plugin index is malformed")
	// ErrTrustRoot: the pinned trusted root is missing, unparseable, or expired.
	ErrTrustRoot = errors.New("sigstore trusted root unavailable")
)

// Verifier performs keyless signature verification against the pinned trusted
// root. It is constructed once (parsing the root is non-trivial) and reused.
type Verifier struct {
	verifier *sgverify.Verifier
}

// NewVerifier builds a Verifier over the embedded public-good trusted root with
// the production keyless posture: Fulcio cert chain + an SCT (CT-log proof) +
// Rekor transparency-log inclusion + an observed timestamp. This matches the
// hub's NewSigstoreVerifier (sctThreshold=1) byte-for-byte so the two agree on
// what a valid signature is.
func NewVerifier() (*Verifier, error) {
	tm, err := root.NewTrustedRootFromJSON(embeddedTrustedRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: parse embedded trusted root: %v", ErrTrustRoot, err)
	}
	return newVerifier(tm, 1)
}

// newVerifier is the test-friendly constructor: it takes any TrustedMaterial
// (so unit tests pass a VirtualSigstore) and an SCT threshold (tests pass 0 —
// VirtualSigstore certs carry no embedded SCT; production passes 1).
func newVerifier(tm root.TrustedMaterial, sctThreshold int) (*Verifier, error) {
	opts := []sgverify.VerifierOption{
		sgverify.WithTransparencyLog(1),
		sgverify.WithObserverTimestamps(1),
	}
	if sctThreshold > 0 {
		opts = append(opts, sgverify.WithSignedCertificateTimestamps(sctThreshold))
	}
	v, err := sgverify.NewVerifier(tm, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: build verifier: %v", ErrTrustRoot, err)
	}
	return &Verifier{verifier: v}, nil
}

// verifySignature parses the bundle JSON and verifies it against the index
// digest, returning the canonical keyless signer identity. The crypto core is
// verifyEntity, split out so unit tests can drive it with a VirtualSigstore
// TestEntity without bundle serialization (mirrors the hub).
func (v *Verifier) verifySignature(ctx context.Context, bundleJSON []byte, indexDigest string) (string, error) {
	if len(bundleJSON) == 0 {
		return "", ErrUnsigned
	}
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return "", fmt.Errorf("%w: parse signature bundle: %v", ErrSignatureInvalid, err)
	}
	return v.verifyEntity(ctx, &b, indexDigest)
}

// verifyEntity verifies a SignedEntity against the index digest and returns the
// canonical keyless signer identity. It enforces the cryptographic floor; the
// identity POLICY (TOFU) is applied by the caller against the returned id, so
// this stays a pure crypto check (mirrors the hub's verifyEntity using
// WithoutIdentitiesUnsafe — identity is pinned out-of-band, not at verify time).
//
// Verification is offline and deterministic (the trusted root is embedded; no
// network I/O or blocking I/O occurs), so no timeout is needed here — a cheap
// ctx.Err() pre-check is sufficient to respect cancellation.
func (v *Verifier) verifyEntity(ctx context.Context, entity sgverify.SignedEntity, indexDigest string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	digestBytes, err := hex.DecodeString(strings.TrimPrefix(indexDigest, "sha256:"))
	if err != nil || len(digestBytes) != sha256.Size {
		return "", fmt.Errorf("%w: invalid index digest %q", ErrSignatureInvalid, indexDigest)
	}
	policy := sgverify.NewPolicy(
		sgverify.WithArtifactDigest("sha256", digestBytes),
		sgverify.WithoutIdentitiesUnsafe(),
	)
	return v.runVerify(entity, policy)
}

func (v *Verifier) runVerify(entity sgverify.SignedEntity, policy sgverify.PolicyBuilder) (string, error) {
	result, err := v.verifier.Verify(entity, policy)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	if result.Signature == nil || result.Signature.Certificate == nil {
		return "", fmt.Errorf("%w: verified signature carries no certificate identity (key-based signing is not accepted for plugins)", ErrSignatureInvalid)
	}
	cert := result.Signature.Certificate
	if cert.Extensions.Issuer == "" || cert.SubjectAlternativeName == "" {
		return "", fmt.Errorf("%w: verified certificate missing OIDC issuer or SAN", ErrSignatureInvalid)
	}
	return identity.CanonicalKeylessIdentity(cert.Extensions.Issuer, cert.SubjectAlternativeName), nil
}
