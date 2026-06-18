package oci

import (
	"bytes"
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"
	"oras.land/oras-go/v2"
)

// Public-good Sigstore endpoints. The hub verifies plugin signatures against the
// embedded public-good trusted root, so plugins MUST be signed against these —
// NOT against any private/dev Fulcio. The signing IDENTITY (the OIDC ID token
// passed to Fulcio) must therefore come from a public-good-Fulcio-trusted issuer
// (GitHub Actions, Google, the interactive sigstore Dex) — it is a DIFFERENT
// token from the grc.store registry/hub bearer (Keycloak), which only authorizes
// push + /sync. See SignerOptions.IDToken.
const (
	publicGoodFulcioURL = "https://fulcio.sigstore.dev"
	publicGoodRekorURL  = "https://rekor.sigstore.dev"
)

// SignerOptions configures keyless signing of an assembled index.
type SignerOptions struct {
	// IDToken is the OIDC identity token Fulcio mints the signing certificate
	// from. It MUST be from a public-good-Fulcio-trusted issuer (GitHub Actions
	// OIDC with audience "sigstore", or an interactive sigstore login) — NOT the
	// Keycloak registry bearer. Required.
	IDToken string
	// FulcioURL / RekorURL override the public-good endpoints (testing only).
	FulcioURL string
	RekorURL  string
}

// SignedBundle holds the keyless signature bundle for an index, ready to attach
// as an OCI referrer. It is the result of SignIndex.
type SignedBundle struct {
	// JSON is the vnd.dev.sigstore.bundle.v0.3+json bytes (the layer content).
	JSON []byte
}

// NewSignedBundle wraps raw bundle JSON for AttachSignature (e.g. bytes produced
// out-of-band, or in tests).
func NewSignedBundle(json []byte) *SignedBundle { return &SignedBundle{JSON: json} }

// SignIndex produces a keyless Sigstore bundle over the assembled index's bytes
// (whose sha256 IS the index digest the bundle is bound to, matching the verify
// policy's WithArtifactDigest). It signs against public-good Fulcio/Rekor using
// the provided OIDC ID token. It does NOT push anything — AttachSignature does.
func SignIndex(ctx context.Context, idx *AssembledIndex, opts SignerOptions) (*SignedBundle, error) {
	if opts.IDToken == "" {
		return nil, fmt.Errorf("a signing OIDC ID token is required (public-good Fulcio identity; distinct from the registry login)")
	}
	fulcioURL := opts.FulcioURL
	if fulcioURL == "" {
		fulcioURL = publicGoodFulcioURL
	}
	rekorURL := opts.RekorURL
	if rekorURL == "" {
		rekorURL = publicGoodRekorURL
	}

	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral signing key: %w", err)
	}

	content := &sign.PlainData{Data: idx.Index.Data}
	bopts := sign.BundleOptions{
		Context:             ctx,
		CertificateProvider: sign.NewFulcio(&sign.FulcioOptions{BaseURL: fulcioURL}),
		CertificateProviderOptions: &sign.CertificateProviderOptions{
			IDToken: opts.IDToken,
		},
		TransparencyLogs: []sign.Transparency{
			sign.NewRekor(&sign.RekorOptions{BaseURL: rekorURL}),
		},
	}

	pb, err := sign.Bundle(content, keypair, bopts)
	if err != nil {
		return nil, fmt.Errorf("keyless signing the index against %s: %w", fulcioURL, err)
	}
	return marshalBundle(pb)
}

// marshalBundle serializes a protobundle.Bundle to the canonical
// vnd.dev.sigstore.bundle.v0.3+json JSON (protojson, as cosign emits).
func marshalBundle(pb *protobundle.Bundle) (*SignedBundle, error) {
	data, err := protojson.Marshal(pb)
	if err != nil {
		return nil, fmt.Errorf("marshalling signature bundle: %w", err)
	}
	return &SignedBundle{JSON: data}, nil
}

// SignAndAttach signs the assembled index against public-good Fulcio/Rekor with
// the given signing ID token and attaches the bundle as the index's OCI referrer
// in the registry. It builds the same repository the index was pushed to (so the
// referrer lands beside the index). opts carries the registry token for the
// authenticated push of the referrer.
func SignAndAttach(ctx context.Context, idx *AssembledIndex, push PushOptions, signing SignerOptions) error {
	sig, err := SignIndex(ctx, idx, signing)
	if err != nil {
		return err
	}
	repo, err := newPluginRepository(push, idx.Coordinate)
	if err != nil {
		return err
	}
	return AttachSignature(ctx, repo, idx.Index.descriptor(), sig)
}

// AttachSignature pushes the signature bundle as an OCI 1.1 referrer of the
// already-pushed index: a manifest whose subject is the index descriptor,
// artifactType BundleMediaType, with one layer (also BundleMediaType) carrying
// the bundle JSON. This is the exact inverse of pull.go's fetchSignatureBundle.
// target must be the same repository the index was pushed to.
func AttachSignature(ctx context.Context, target oras.Target, indexDesc ocispec.Descriptor, sig *SignedBundle) error {
	// Push the bundle JSON as a blob layer.
	layer := ocispec.Descriptor{
		MediaType: BundleMediaType,
		Digest:    digest.FromBytes(sig.JSON),
		Size:      int64(len(sig.JSON)),
	}
	if err := target.Push(ctx, layer, bytes.NewReader(sig.JSON)); err != nil {
		return fmt.Errorf("pushing signature layer: %w", err)
	}

	// Pack a referrer manifest with the index as its subject. PackManifest
	// pushes it; oras computes a minimal empty config.
	_, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, BundleMediaType, oras.PackManifestOptions{
		Subject: &indexDesc,
		Layers:  []ocispec.Descriptor{layer},
	})
	if err != nil {
		return fmt.Errorf("packing signature referrer manifest: %w", err)
	}
	return nil
}
