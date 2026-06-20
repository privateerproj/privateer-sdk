package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

// Fetch caps for the verify walk. A child manifest/config blob is small; the
// binary layer is bounded but generous.
const (
	maxBlobBytes   int64 = 16 << 20  // 16 MiB
	maxBinaryBytes int64 = 500 << 20 // 500 MiB
)

// IdentityPolicy decides whether a verified signer identity is acceptable.
// Camp (b) TOFU: on first install the pin is empty → accept and the caller
// records the returned identity; on update the pin is the previously-recorded
// identity → require equality.
type IdentityPolicy struct {
	// PinnedIdentity is the previously-recorded canonical identity, or "" on
	// first install.
	PinnedIdentity string
}

// check returns nil if id satisfies the policy, else ErrIdentityMismatch.
func (p IdentityPolicy) check(id string) error {
	if p.PinnedIdentity == "" {
		return nil // first install — TOFU accepts any valid keyless identity
	}
	if id != p.PinnedIdentity {
		return fmt.Errorf("%w: got %q, pinned %q", ErrIdentityMismatch, id, p.PinnedIdentity)
	}
	return nil
}

// VerifiedPlugin is the trusted result of a successful §6 verification: the
// bytes to install, the name to install them under, and the provenance to
// record. Nothing here is derived from an unsigned source — entrypoint/protocol
// come from the digest-checked config blob, the binary from the digest-checked
// layer, the identity from the verified cert.
type VerifiedPlugin struct {
	Coordinate     string
	Version        string
	IndexDigest    string // sha256:... — what was signed; record for drift detection
	SignerIdentity string // canonical keyless identity; pin on first install
	Entrypoint     string // go-plugin discovery name (binary filename) — from the signed config
	Protocol       string
	OS             string
	Arch           string
	Binary         []byte // the verified binary bytes to write +x under Entrypoint
}

// Index runs the full §6 contract over a fetched (untrusted) index and returns
// the VerifiedPlugin for the host platform, or a named fail-closed error. The
// order is load-bearing: verify the signature and identity BEFORE trusting any
// index bytes, then walk index → child → config/layer → bytes, checking every
// digest. Any failure aborts; nothing degrades to an unverified copy.
//
// Multiple signature bundles are tried in order (they accumulate because
// AttachSignature never removes prior referrers and re-signing is
// content-addressed/idempotent). The first bundle that passes BOTH signature
// verification and the identity policy proceeds to the walk. If no bundle
// passes, the errors are collected and the first error's sentinel is preserved
// so errors.Is(err, ErrSignatureInvalid) / ErrIdentityMismatch still work.
func (v *Verifier) Index(ctx context.Context, fetched *oci.FetchedIndex, policy IdentityPolicy) (*VerifiedPlugin, error) {
	if fetched == nil {
		return nil, fmt.Errorf("%w: nil fetched index", ErrMalformedIndex)
	}
	// 1. Verify the index signature against the index digest (keyless: Fulcio
	//    chain + SCT + Rekor inclusion, offline against the pinned root).
	//    Re-signing accumulates bundles without removing prior ones, so we
	//    iterate all bundles and proceed with the first that passes both
	//    cryptographic verification and the identity policy.
	indexDigest := fetched.IndexDescriptor.Digest.String()

	if len(fetched.SignatureBundles) == 0 {
		return nil, ErrUnsigned
	}

	var bundleErrs []error
	for _, bundleJSON := range fetched.SignatureBundles {
		signerIdentity, err := v.verifySignature(ctx, bundleJSON, indexDigest)
		if err != nil {
			bundleErrs = append(bundleErrs, err)
			continue
		}
		if err := policy.check(signerIdentity); err != nil {
			bundleErrs = append(bundleErrs, err)
			continue
		}
		// This bundle passed both crypto verification and the identity policy —
		// proceed to the walk. The identity check here is load-bearing, not just a
		// pre-filter: on a mismatch the loop `continue`s to the NEXT bundle, so a
		// plugin carrying multiple signatures is accepted as long as one matches the
		// pinned identity. walkVerifiedIndex re-checks (step 2) so the test entrypoint
		// that calls it directly is gated identically.
		return v.walkVerifiedIndex(ctx, fetched, signerIdentity, policy)
	}

	// No bundle passed. If the index carried more signature referrers than were
	// inspected, say so explicitly: a registry that floods junk referrers must
	// not be able to mask a real signature as a plain verification failure or
	// (worse) as "unsigned". This is diagnosable and fails closed.
	if fetched.SignaturesTruncated {
		return nil, fmt.Errorf("%w: no valid signature among the first %d referrers, but the index carries more (a valid signature may exist beyond the inspection limit; the registry may be flooding referrers)",
			ErrSignatureInvalid, len(bundleErrs))
	}
	// For a single bundle the error is returned directly so
	// errors.Is(err, ErrSignatureInvalid) / ErrIdentityMismatch work as before.
	// For multiple bundles, errors.Join wraps all of them — errors.Is still
	// walks the joined tree so sentinels remain detectable.
	if len(bundleErrs) == 1 {
		return nil, bundleErrs[0]
	}
	return nil, fmt.Errorf("no valid signature bundle found (tried %d): %w", len(bundleErrs), errors.Join(bundleErrs...))
}

// walkVerifiedIndex runs steps 2-8 after the signature has been verified and the
// signer identity extracted. Split from Index so unit tests can drive the full
// walk with a VirtualSigstore TestEntity (verifying via verifyEntity) without
// serializing a bundle.
func (v *Verifier) walkVerifiedIndex(ctx context.Context, fetched *oci.FetchedIndex, signerIdentity string, policy IdentityPolicy) (*VerifiedPlugin, error) {
	indexDigest := fetched.IndexDescriptor.Digest.String()

	// 2. Identity policy (camp (b) TOFU). A valid signature from an unexpected
	//    identity is still an attack; this is the gate that stops it.
	if err := policy.check(signerIdentity); err != nil {
		return nil, err
	}

	// 3. Index integrity: the bytes we parse must hash to the digest we verified.
	if err := checkDigest(fetched.IndexDescriptor.Digest, fetched.IndexBytes, "index"); err != nil {
		return nil, err
	}
	var index ocispec.Index
	if err := json.Unmarshal(fetched.IndexBytes, &index); err != nil {
		return nil, fmt.Errorf("%w: parse index: %v", ErrMalformedIndex, err)
	}
	if index.MediaType != ocispec.MediaTypeImageIndex {
		return nil, fmt.Errorf("%w: top-level media type %q is not an image index", ErrMalformedIndex, index.MediaType)
	}
	if len(index.Manifests) == 0 {
		return nil, fmt.Errorf("%w: index lists no platform children", ErrMalformedIndex)
	}

	// 4. Select the child for the host os/arch using the INDEX DESCRIPTOR's
	//    platform (the hub's preferred source). Platform-unavailable is a
	//    first-class checked condition, not a nil-deref.
	childDesc, err := selectHostChild(index.Manifests)
	if err != nil {
		return nil, err
	}

	// 5. index → child: fetch + digest-check the child manifest.
	childBytes, err := oci.FetchBytes(ctx, fetched.Target(), childDesc, maxBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch child manifest: %v", ErrDigestMismatch, err)
	}
	if err := checkDigest(childDesc.Digest, childBytes, "child manifest"); err != nil {
		return nil, err
	}
	var child ocispec.Manifest
	if err := json.Unmarshal(childBytes, &child); err != nil {
		return nil, fmt.Errorf("%w: parse child manifest: %v", ErrMalformedIndex, err)
	}

	// 6. child → config: exactly the plugin config media type; fetch + check.
	if child.Config.MediaType != oci.MediaTypePluginConfig {
		return nil, fmt.Errorf("%w: child config media type %q is not %q", ErrMalformedIndex, child.Config.MediaType, oci.MediaTypePluginConfig)
	}
	configBytes, err := oci.FetchBytes(ctx, fetched.Target(), child.Config, maxBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch config blob: %v", ErrDigestMismatch, err)
	}
	if err := checkDigest(child.Config.Digest, configBytes, "config blob"); err != nil {
		return nil, err
	}
	var cfg pluginspec.Config
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("%w: parse config blob: %v", ErrMalformedIndex, err)
	}

	// 7. child → layer: exactly one binary layer; fetch + digest-check the bytes.
	layerDesc, err := singleBinaryLayer(child.Layers)
	if err != nil {
		return nil, err
	}
	binBytes, err := oci.FetchBytes(ctx, fetched.Target(), layerDesc, maxBinaryBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch binary layer: %v", ErrDigestMismatch, err)
	}
	if err := checkDigest(layerDesc.Digest, binBytes, "binary layer"); err != nil {
		return nil, err
	}

	// 8. Trusted facts: entrypoint/protocol from the digest-checked config; the
	//    config version and plugin coordinate must equal what was requested.
	//    The coordinate check closes a substitution window: a validly-signed
	//    index for a *different* plugin served under the requested coordinate
	//    would otherwise pass verification — AssembleIndex writes cfg.Plugin
	//    into the config blob precisely so this check can catch it.
	if cfg.Entrypoint == "" {
		return nil, fmt.Errorf("%w: config has no entrypoint", ErrMalformedIndex)
	}
	if cfg.Version != fetched.Version {
		return nil, fmt.Errorf("%w: config version %q != requested tag %q", ErrMalformedIndex, cfg.Version, fetched.Version)
	}
	if cfg.Plugin != fetched.Coordinate {
		return nil, fmt.Errorf("%w: config plugin %q != requested coordinate %q", ErrMalformedIndex, cfg.Plugin, fetched.Coordinate)
	}

	osName, arch := childDesc.Platform.OS, childDesc.Platform.Architecture
	return &VerifiedPlugin{
		Coordinate:     fetched.Coordinate,
		Version:        fetched.Version,
		IndexDigest:    indexDigest,
		SignerIdentity: signerIdentity,
		Entrypoint:     cfg.Entrypoint,
		Protocol:       cfg.Protocol,
		OS:             osName,
		Arch:           arch,
		Binary:         binBytes,
	}, nil
}

// selectHostChild returns the index child descriptor matching the running
// host's os/arch, preferring the descriptor's own platform. Returns
// ErrPlatformUnavailable (a checked condition) when none matches.
func selectHostChild(children []ocispec.Descriptor) (ocispec.Descriptor, error) {
	var available []string
	for _, c := range children {
		if c.Platform == nil || c.Platform.OS == "" || c.Platform.Architecture == "" {
			continue // a child without descriptor platform can't be host-matched
		}
		if c.Platform.OS == runtime.GOOS && c.Platform.Architecture == runtime.GOARCH {
			return c, nil
		}
		available = append(available, c.Platform.OS+"/"+c.Platform.Architecture)
	}
	return ocispec.Descriptor{}, fmt.Errorf("%w: host %s/%s (available: %v)", ErrPlatformUnavailable, runtime.GOOS, runtime.GOARCH, available)
}

// singleBinaryLayer returns the one plugin-binary layer, or an error if there
// is not exactly one (the grc.store contract requires exactly one).
func singleBinaryLayer(layers []ocispec.Descriptor) (ocispec.Descriptor, error) {
	var found *ocispec.Descriptor
	for i := range layers {
		if layers[i].MediaType == oci.MediaTypePluginBinary {
			if found != nil {
				return ocispec.Descriptor{}, fmt.Errorf("%w: more than one binary layer", ErrMalformedIndex)
			}
			found = &layers[i]
		}
	}
	if found == nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: no %q layer", ErrMalformedIndex, oci.MediaTypePluginBinary)
	}
	return *found, nil
}

// checkDigest fails closed (ErrDigestMismatch) if data does not hash to want.
func checkDigest(want digest.Digest, data []byte, what string) error {
	got := digest.FromBytes(data)
	if got != want {
		return fmt.Errorf("%w: %s digest %s != expected %s", ErrDigestMismatch, what, got, want)
	}
	return nil
}
