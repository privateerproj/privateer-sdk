package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/revanite-io/grc-store-protocol/mediatype"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// BundleMediaType is the artifactType/mediaType of the Sigstore v0.3 bundle
// stored as an OCI 1.1 referrer of the plugin index — re-exported from the
// shared protocol contract (single source of truth; ADR-0035).
const BundleMediaType = mediatype.SigstoreBundle

// maxBlobBytes caps any single fetched blob (manifest, config, bundle). The
// binary layer is read by the verify walk with its own (larger) cap.
const maxBlobBytes int64 = 16 << 20 // 16 MiB

// maxSignatureReferrers caps how many signature referrers we fetch + verify.
// The referrer list is registry-controlled; this bounds the resource cost of a
// re-signed (or maliciously over-signed) index. A few signings is realistic, so
// the legitimate signature is virtually always within this window — but when the
// cap IS hit without finding a valid bundle, fetchSignatureBundle flags it so
// verify can report a distinct "signatures beyond inspection limit" error rather
// than an indistinguishable "unsigned" (a registry that crowds in junk referrers
// must not be able to silently mask a valid signature as absent).
const maxSignatureReferrers = 32

// FetchedIndex is the raw, NOT-yet-verified result of pulling a plugin index:
// the index descriptor (its digest), the index bytes, and the signature bundles
// discovered as referrers (nil when the index carries no signature referrer —
// that is "unsigned", a verify-time concern, not a fetch error; multiple entries
// when the index has been re-signed, since AttachSignature never removes prior
// referrers and pushes are content-addressed/idempotent). Everything here is
// untrusted until verify runs.
type FetchedIndex struct {
	Coordinate      string
	Version         string
	IndexDescriptor ocispec.Descriptor
	IndexBytes      []byte
	// SignatureBundles holds the raw Sigstore v0.3 bundle JSON for each
	// signature referrer. Nil (or empty) means unsigned. Multiple entries arise
	// when the index has been re-signed: re-signing pushes a new bundle without
	// removing the old one, so bundles accumulate across signing runs.
	SignatureBundles [][]byte
	// SignaturesTruncated is true when the index carried more signature referrers
	// than maxSignatureReferrers, so not all were inspected. Verify uses it to
	// distinguish "no valid signature within the inspected set" from "genuinely
	// unsigned" — a registry that floods junk referrers must not be able to mask a
	// real signature as absent.
	SignaturesTruncated bool
	// target is retained so the verify walk can fetch child manifests, config
	// blobs, and the binary layer from the same source.
	target oras.ReadOnlyTarget
}

// Target exposes the read-only target for the verify walk to fetch children.
func (f *FetchedIndex) Target() oras.ReadOnlyTarget { return f.target }

// NewFetchedIndex builds a FetchedIndex from an already-open read-only target
// (e.g. an in-memory store or an OCI layout) instead of a live registry pull.
// It is used by tests and by any caller that has the index bytes + bundles in
// hand. No verification is performed — the result is untrusted input to
// verify.Index, exactly like PullIndex's output.
func NewFetchedIndex(coordinate, version string, indexDesc ocispec.Descriptor, indexBytes []byte, signatureBundles [][]byte, target oras.ReadOnlyTarget) *FetchedIndex {
	return &FetchedIndex{
		Coordinate:       coordinate,
		Version:          version,
		IndexDescriptor:  indexDesc,
		IndexBytes:       indexBytes,
		SignatureBundles: signatureBundles,
		target:           target,
	}
}

// PullOptions configures an anonymous plugin-index pull.
type PullOptions struct {
	RegistryHost string
	PlainHTTP    bool
}

// ErrNotIndex is returned when a plugin tag resolves to something other than an
// OCI image index. The grc.store contract requires the tag to be an index even
// for a single platform.
var ErrNotIndex = errors.New("plugin tag did not resolve to an OCI image index")

// registryHTTPClient bounds the registry transport: a stalled registry must
// fail, not hang `pvtr install` forever. No overall request timeout — binary
// layers can legitimately take minutes on slow links — but the server must
// start responding promptly.
func registryHTTPClient() *http.Client {
	return &http.Client{
		Transport: retry.NewTransport(&http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}),
	}
}

// PullIndex resolves <host>/<ns>/plugins/<id>:<version>, fetches the index, and
// discovers the signature bundle referrer. It performs NO verification — the
// caller passes the result to verify.Index. The anonymous bearer-token dance is
// handled by oras-go.
func PullIndex(ctx context.Context, coordinate, version string, opts PullOptions) (*FetchedIndex, error) {
	repo, err := newPluginRepository(PushOptions{
		RegistryHost: opts.RegistryHost,
		PlainHTTP:    opts.PlainHTTP,
	}, coordinate)
	if err != nil {
		return nil, err
	}
	// Anonymous pull with bounded transport: the default client has no timeout,
	// so a stalled registry hangs pvtr install forever. registryHTTPClient adds
	// TLS-handshake and response-header deadlines without capping binary-layer
	// transfers (which can legitimately be slow on constrained links).
	repo.Client = &auth.Client{Client: registryHTTPClient(), Cache: auth.NewCache()}

	indexDesc, err := repo.Resolve(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("resolving %s:%s: %w", coordinate, version, err)
	}
	if indexDesc.MediaType != ocispec.MediaTypeImageIndex {
		return nil, fmt.Errorf("%w: tag %s resolved to media type %q", ErrNotIndex, version, indexDesc.MediaType)
	}
	indexBytes, err := FetchBytes(ctx, repo, indexDesc, maxBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}

	bundles, truncated, err := fetchSignatureBundle(ctx, repo, indexDesc)
	if err != nil {
		// A transport error during discovery is fatal: we can't claim
		// "unsigned" if we couldn't look (fail-closed).
		return nil, fmt.Errorf("discovering signature: %w", err)
	}

	return &FetchedIndex{
		Coordinate:          coordinate,
		Version:             version,
		IndexDescriptor:     indexDesc,
		IndexBytes:          indexBytes,
		SignatureBundles:    bundles,
		SignaturesTruncated: truncated,
		target:              repo,
	}, nil
}

// FetchSignature discovers all Sigstore bundles attached to an index in a
// target and returns their raw JSON bytes (nil when unsigned). Re-signing
// accumulates referrers without removing prior ones, so multiple bundles can be
// present; the verify walk tries each and proceeds with the first that passes.
// Exported so a caller that already has an index descriptor + target (e.g.
// re-verification, tests) can run the same discovery PullIndex does. It is the
// exact inverse of AttachSignature. The truncation flag is internal to the pull
// path, so this re-verification helper drops it.
func FetchSignature(ctx context.Context, target oras.ReadOnlyTarget, indexDesc ocispec.Descriptor) ([][]byte, error) {
	bundles, _, err := fetchSignatureBundle(ctx, target, indexDesc)
	return bundles, err
}

// fetchSignatureBundle discovers all Sigstore v0.3 bundles attached to the
// index as OCI referrers and returns their raw JSON bytes, or (nil, false, nil)
// when none exist. Re-signing is additive — AttachSignature never removes prior
// referrers, so a re-signed index accumulates bundles across signing runs.
// The verify walk iterates all bundles, trying each against the identity policy;
// the first that passes both signature verification and the policy proceeds.
// Mirrors the hub's fetchSignatureBundle shape but collects ALL referrers. The
// returned bool is true when more referrers existed than were inspected (the cap
// was hit), so verify can tell "no valid signature found within the inspected
// set" apart from "genuinely unsigned".
func fetchSignatureBundle(ctx context.Context, target oras.ReadOnlyTarget, indexDesc ocispec.Descriptor) ([][]byte, bool, error) {
	gs, ok := target.(content.ReadOnlyGraphStorage)
	if !ok {
		// Can't discover referrers → can't find a signature → treat as unsigned.
		return nil, false, nil
	}
	refs, err := registry.Referrers(ctx, gs, indexDesc, BundleMediaType)
	if err != nil {
		return nil, false, fmt.Errorf("listing referrers: %w", err)
	}
	if len(refs) == 0 {
		return nil, false, nil
	}

	// Cap how many referrers we will fetch + verify. The referrer list is
	// registry-controlled; without a bound a malicious registry could attach
	// thousands of bundles, forcing N×maxBlobBytes of buffering and N expensive
	// sigstore verifications (a resource-exhaustion DoS). A handful of
	// re-signings is the realistic ceiling. We do NOT silently drop the excess:
	// truncated is reported up so a flood of junk referrers crowding out a valid
	// signature surfaces as a distinct error, not a false "unsigned".
	truncated := len(refs) > maxSignatureReferrers
	if truncated {
		refs = refs[:maxSignatureReferrers]
	}

	// Collect one bundle JSON per referrer. A referrer manifest may carry
	// multiple layers but only one BundleMediaType layer per signing run. A
	// single broken/poisoned referrer must NOT make a validly-signed plugin
	// uninstallable, so we skip referrers that fail to fetch or parse and only
	// surface an error when we found no usable bundle at all (fail-closed: a
	// discovery error with zero bundles is reported, never silently downgraded
	// to "unsigned").
	var bundles [][]byte
	var lastErr error
	for _, ref := range refs {
		manifestBytes, err := FetchBytes(ctx, target, ref, maxBlobBytes)
		if err != nil {
			lastErr = fmt.Errorf("fetching signature manifest: %w", err)
			continue
		}
		var m ocispec.Manifest
		if err := json.Unmarshal(manifestBytes, &m); err != nil {
			lastErr = fmt.Errorf("parsing signature manifest: %w", err)
			continue
		}
		for _, layer := range m.Layers {
			if layer.MediaType == BundleMediaType {
				data, err := FetchBytes(ctx, target, layer, maxBlobBytes)
				if err != nil {
					lastErr = fmt.Errorf("fetching bundle layer: %w", err)
					break
				}
				bundles = append(bundles, data)
				break // each referrer manifest carries at most one bundle layer
			}
		}
	}
	if len(bundles) == 0 {
		// No usable bundle. If every referrer failed to fetch, that is a
		// discovery error (fail-closed); if there simply were no bundle layers,
		// the index is unsigned (nil, nil → verify returns ErrUnsigned). The
		// truncation flag still propagates so verify can distinguish a flooded
		// referrer list from a genuinely unsigned index.
		return nil, truncated, lastErr
	}
	return bundles, truncated, nil
}

// FetchBytes fetches a descriptor's content (capped). oras's Fetch
// content-verifies against the descriptor digest internally; the verify walk
// ALSO re-checks digests explicitly so a mismatch surfaces as a named
// ErrDigestMismatch (defense-in-depth, not redundancy theater). Exported so the
// verify package can fetch children of an already-verified index.
//
// When the descriptor declares a size that already exceeds the limit, the fetch
// is skipped entirely — we know it will be over-cap without touching the wire.
// When the content arrives, we read limit+1 bytes so that an over-cap stream
// returns an explicit size error rather than silent truncation (which would
// surface later as a spurious ErrDigestMismatch, a tampering-shaped signal for
// what is actually a size-limit condition).
func FetchBytes(ctx context.Context, target content.Fetcher, desc ocispec.Descriptor, limit int64) ([]byte, error) {
	if desc.Size > limit {
		return nil, fmt.Errorf("content exceeds maximum allowed size (%d bytes)", limit)
	}
	rc, err := target.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("content exceeds maximum allowed size (%d bytes)", limit)
	}
	return data, nil
}
