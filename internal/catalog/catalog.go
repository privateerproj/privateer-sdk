// Package catalog fetches Gemara control catalogs from grc.store, verifies
// them, and caches them where plugins read them at run time. `pvtr install`
// uses it to install the catalogs a plugin declares; `pvtr publish` uses it to
// read each declared catalog while building the plugin's evaluates linkage.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Fetch pulls <namespace>/<id>:<version> from the hub's registry and verifies
// it end-to-end: keyless signature against the pinned trust root, signer
// identity against the one the hub verified at ingest, and the manifest digest
// against the hub's record. It fails closed; a catalog the hub does not know is
// ErrCatalogNotFound. Warnings are written to w.
func Fetch(ctx context.Context, w io.Writer, hub *oci.Client, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	// The version is the whole of a coordinate's meaning here: it picks the OCI
	// tag to pull and the cache file to write. Rejecting an empty one at this
	// single choke point means no path can quietly resolve "whatever is latest
	// today" into the cache a plugin pinned to an exact version will read.
	if c.Version == "" {
		return nil, fmt.Errorf("catalog %s has no version; name it as <namespace>/<id>@<version>", c.Repository())
	}

	detail, err := hub.GetCatalogDetails(ctx, c.Namespace, c.ID)
	if err != nil {
		return nil, err
	}
	release, err := detail.Release(c.Version)
	if err != nil {
		return nil, err
	}

	host, plainHTTP, err := hub.Registry(ctx)
	if err != nil {
		return nil, err
	}
	fetched, err := oci.PullManifest(ctx, c.Repository(), release.Version, oci.PullOptions{RegistryHost: host, PlainHTTP: plainHTTP})
	if err != nil {
		return nil, fmt.Errorf("pulling catalog: %w", err)
	}
	if err := checkHubDigest(w, c, release.ManifestDigest, fetched.IndexDescriptor.Digest.String()); err != nil {
		return nil, err
	}

	verifier, err := verify.NewVerifier()
	if err != nil {
		return nil, fmt.Errorf("initializing verifier: %w", err)
	}
	// TODO: the hub's signer identity is taken as the expected one on every
	// fetch. Catalogs have no local trust-on-first-use pin, unlike plugins
	// (internal/install.pinnedIdentityFor), so a hub that changes a catalog's
	// signer for a new version meets no local resistance. Deferred to a
	// follow-up issue.
	verified, err := verifier.Catalog(ctx, fetched, verify.IdentityPolicy{PinnedIdentity: detail.SignerIdentity})
	if err != nil {
		return nil, fmt.Errorf("verifying %s: %w", c, err)
	}
	return verified, nil
}

// checkHubDigest cross-checks the manifest digest the registry served against
// the one the hub recorded at ingest. A missing hub digest is reported on w
// rather than skipped silently: the plugin path says so out loud, and the same
// hub state must not read as "verified" on one path and "verified plus
// digest-checked" on the other.
func checkHubDigest(w io.Writer, c pluginkit.CatalogCoordinate, hubDigest, registryDigest string) error {
	if hubDigest == "" {
		_, _ = fmt.Fprintf(w, "Warning: hub recorded no manifest digest for %s; skipping registry-divergence cross-check\n", c)
		return nil
	}
	if registryDigest != hubDigest {
		return fmt.Errorf("registry diverged from hub for %s: registry manifest digest %s != hub-recorded %s — refusing to use it",
			c, registryDigest, hubDigest)
	}
	return nil
}

// fetchFunc is how install obtains and verifies one catalog. Install supplies
// Fetch; tests supply a stub, so the cache-write path can run without a hub or
// a real signature.
type fetchFunc func(context.Context, io.Writer, *oci.Client, pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error)

// Install caches every coordinate under binariesDir (see
// pluginkit.CatalogCachePath), fetching and verifying the ones not already
// there; a published version is immutable, so a cached file is never
// re-fetched. Any verification failure aborts. Progress is written to w.
//
// skipUnpublished says what a catalog the hub does not know means, which
// differs by caller. `pvtr install <coordinate>` passes true: its coordinates
// come from an installed plugin's signed evaluates list, where an older
// plugin's entries can name a catalog it embeds itself and never published, so
// such a catalog is reported on w and skipped. `pvtr install --local` passes
// false: its coordinates come only from AddCatalogs, so every one of them names
// a catalog that is supposed to be on grc.store, and a missing one is a failure
// the user must see now rather than as a run-time error later.
func Install(ctx context.Context, w io.Writer, hub *oci.Client, binariesDir string, coords []pluginkit.CatalogCoordinate, skipUnpublished bool) error {
	return install(ctx, w, hub, binariesDir, coords, skipUnpublished, Fetch)
}

func install(ctx context.Context, w io.Writer, hub *oci.Client, binariesDir string, coords []pluginkit.CatalogCoordinate, skipUnpublished bool, fetch fetchFunc) error {
	for _, c := range coords {
		path := pluginkit.CatalogCachePath(binariesDir, c)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		verified, err := fetch(ctx, w, hub, c)
		if err != nil {
			if skipUnpublished && errors.Is(err, oci.ErrCatalogNotFound) {
				_, _ = fmt.Fprintf(w, "Warning: catalog %s is not published on grc.store; skipping (the plugin must carry its own copy)\n", c)
				continue
			}
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating catalog cache dir: %w", err)
		}
		if err := utils.WriteFileAtomic(path, verified.YAML, 0o644); err != nil {
			return fmt.Errorf("writing catalog %s: %w", c, err)
		}
		_, _ = fmt.Fprintf(w, "Installed catalog %s (signed by %s)\n", c, verified.SignerIdentity)
	}
	return nil
}
