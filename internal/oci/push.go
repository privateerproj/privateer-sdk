package oci

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ReservedPluginSegment is the reserved repo path segment distinguishing a
// plugin OCI repository (<ns>/plugins/<plugin_id>) from a Gemara catalog repo
// under the same namespace (grc.store ADR-0034 decision 5). Mirrors the hub's
// store.ReservedPluginSegment. It must stay OCI-distribution-spec-valid (a path
// component must begin with [a-z0-9]); the earlier "_plugins" value was illegal
// and was changed to "plugins".
const ReservedPluginSegment = "plugins"

// PushOptions configures a push to an OCI registry.
type PushOptions struct {
	// RegistryHost is the scheme-stripped registry host (e.g. "localhost:5050"
	// or "oci.grc.store"), as returned by Discovery.RegistryHost.
	RegistryHost string
	// PlainHTTP forces http:// instead of https:// — required for the local dev
	// zot at localhost:5050. Prod (oci.grc.store) is https, so this defaults
	// false.
	PlainHTTP bool
	// Client is the auth-capable HTTP client. Nil uses oras's anonymous default
	// (fine for a no-auth registry or anonymous pull-only checks; a push to a
	// bearer-gated registry needs a credentialed client).
	Client remote.Client
	// RegistryToken is a zot registry token (from MintRegistryToken) for an
	// authenticated push to a bearer-gated registry. When set (and Client is
	// nil), the push client sends it directly to the registry as the access
	// token — oras does no /v2/token exchange because we already did it. Empty
	// keeps the anonymous default (backward-compatible).
	RegistryToken string
}

// Push writes an AssembledIndex to the registry under
// <host>/<coordinate-namespaced repo>:<version> and tags the index. It walks
// leaves-first: config + binary blobs, then child manifests, then the index.
// It is content-addressed throughout, so re-pushing identical bytes is a no-op
// at the registry. Signing is NOT done here — the index digest this produces is
// what cosign signs (separately) and what the manifest records.
func Push(ctx context.Context, idx *AssembledIndex, opts PushOptions) (indexDigest string, err error) {
	repo, err := newPluginRepository(opts, idx.Coordinate)
	if err != nil {
		return "", err
	}
	return idx.PushTo(ctx, repo)
}

// PushTo writes the assembled index to any oras Target (a remote repository, or
// an in-memory store for tests), leaves-first: config + binary blobs, then child
// manifests, then the tagged index. Content-addressed throughout, so re-pushing
// identical bytes is idempotent. Returns the index digest (what gets signed).
func (idx *AssembledIndex) PushTo(ctx context.Context, target oras.Target) (string, error) {
	for _, b := range idx.Blobs {
		if err := pushBlobTo(ctx, target, b); err != nil {
			return "", fmt.Errorf("pushing blob %s: %w", b.Digest, err)
		}
	}
	for _, m := range idx.Manifests {
		if err := pushBlobTo(ctx, target, m); err != nil {
			return "", fmt.Errorf("pushing child manifest %s: %w", m.Digest, err)
		}
	}
	if err := pushBlobTo(ctx, target, idx.Index); err != nil {
		return "", fmt.Errorf("pushing index: %w", err)
	}
	if err := target.Tag(ctx, idx.Index.descriptor(), idx.Version); err != nil {
		return "", fmt.Errorf("tagging index %s: %w", idx.Version, err)
	}
	return idx.Index.Digest.String(), nil
}

// pushBlobTo pushes one content-addressed blob to a generic target, treating
// AlreadyExists as success.
func pushBlobTo(ctx context.Context, target oras.Target, b blob) error {
	desc := b.descriptor()
	exists, err := target.Exists(ctx, desc)
	if err == nil && exists {
		return nil
	}
	return target.Push(ctx, desc, bytes.NewReader(b.Data))
}

// newPluginRepository builds the oras repository client for a plugin
// coordinate. The repo path is "<namespace>/plugins/<plugin_id>" — the
// `plugins` segment is reserved by grc.store to distinguish plugin repos from
// catalog repos under the same namespace (ADR-0034 decision 5; `plugins` is
// also a reserved org slug, so the first segment can never collide with it).
func newPluginRepository(opts PushOptions, coordinate string) (*remote.Repository, error) {
	if opts.RegistryHost == "" {
		return nil, fmt.Errorf("registry host is required")
	}
	ns, id, ok := splitCoordinate(coordinate)
	if !ok {
		return nil, fmt.Errorf("invalid coordinate %q: want <namespace>/<plugin_id>", coordinate)
	}
	// `plugins` is an OCI-distribution-spec-valid path component, so the
	// ordinary string reference path works (no struct-built Reference needed —
	// the earlier _plugins segment forced that workaround, which the maintainers
	// changed away from precisely because a leading-underscore segment is
	// illegal and oras-go re-validates the name inside Resolve/Fetch/Referrers).
	ref := opts.RegistryHost + "/" + pluginRepoPath(ns, id)
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("building repository client for %s: %w", ref, err)
	}
	repo.PlainHTTP = opts.PlainHTTP
	switch {
	case opts.Client != nil:
		repo.Client = opts.Client
	case opts.RegistryToken != "":
		// Authenticated push: hand oras the registry token we already minted at
		// /v2/token, scoped to this exact repo, so it goes straight to the
		// registry (no oras-side token exchange). Use registryHTTPClient so push
		// gets the same TLS-handshake and response-header bounds as pull.
		repo.Client = &auth.Client{
			Client: registryHTTPClient(),
			Credential: auth.StaticCredential(opts.RegistryHost, auth.Credential{
				AccessToken: opts.RegistryToken,
			}),
		}
	default:
		// Explicit anonymous client; makes the no-credentials case obvious.
		repo.Client = auth.DefaultClient
	}
	return repo, nil
}

// pluginRepoPath returns the registry repository path "<namespace>/plugins/<id>"
// the hub uses for a plugin. The hub compares this byte-for-byte, so every caller
// (token mint, push, sync) must build it through here.
func pluginRepoPath(ns, id string) string {
	return ns + "/" + ReservedPluginSegment + "/" + id
}

// splitCoordinate splits "<namespace>/<plugin_id>" into its parts.
func splitCoordinate(coordinate string) (ns, id string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(coordinate), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[1], "/") {
		return "", "", false
	}
	return parts[0], parts[1], true
}
