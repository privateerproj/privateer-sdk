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

	ckhub "github.com/gemaraproj/grc-store-clientkit/hub"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Fetch pulls <namespace>/<id>:<version> from the hub's registry and verifies
// it end-to-end: keyless signature against the pinned trust root, signer
// identity against the one the hub verified at ingest, and the manifest digest
// against the hub's record. It fails closed; a catalog the hub does not know
// is ErrCatalogNotFound.
func Fetch(ctx context.Context, hub *oci.Client, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	detail, err := hub.GetCatalogDetails(ctx, c.Namespace, c.ID)
	if err != nil {
		return nil, err
	}
	release, err := detail.Release(c.Version)
	if err != nil {
		return nil, err
	}

	remote, err := ckhub.Discover(ctx, hub.BaseURL())
	if err != nil {
		return nil, fmt.Errorf("hub discovery: %w", err)
	}
	host, plainHTTP, err := ckhub.Registry(remote)
	if err != nil {
		return nil, fmt.Errorf("resolving registry host: %w", err)
	}
	fetched, err := oci.PullManifest(ctx, c.Repository(), release.Version, oci.PullOptions{RegistryHost: host, PlainHTTP: plainHTTP})
	if err != nil {
		return nil, fmt.Errorf("pulling catalog: %w", err)
	}
	if release.ManifestDigest != "" && fetched.IndexDescriptor.Digest.String() != release.ManifestDigest {
		return nil, fmt.Errorf("registry diverged from hub for %s: registry manifest digest %s != hub-recorded %s — refusing to use it",
			c, fetched.IndexDescriptor.Digest, release.ManifestDigest)
	}

	verifier, err := verify.NewVerifier()
	if err != nil {
		return nil, fmt.Errorf("initializing verifier: %w", err)
	}
	// ponytail: the hub's signer identity is the expected one; no local TOFU pin
	// for catalogs yet (add one beside the plugin manifest if the hub is ever
	// not trusted to report it).
	verified, err := verifier.Catalog(ctx, fetched, verify.IdentityPolicy{PinnedIdentity: detail.SignerIdentity})
	if err != nil {
		return nil, fmt.Errorf("verifying %s: %w", c, err)
	}
	return verified, nil
}

// Install caches every coordinate under binariesDir (see
// pluginkit.CatalogCachePath), fetching and verifying the ones not already
// there; a published version is immutable, so a cached file is never
// re-fetched. A catalog the hub does not know is reported on w and skipped —
// an older plugin's evaluates name catalogs it embeds itself — but any
// verification failure aborts. Progress is written to w.
func Install(ctx context.Context, w io.Writer, hub *oci.Client, binariesDir string, coords []pluginkit.CatalogCoordinate) error {
	for _, c := range coords {
		path := pluginkit.CatalogCachePath(binariesDir, c)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		verified, err := Fetch(ctx, hub, c)
		if errors.Is(err, oci.ErrCatalogNotFound) {
			_, _ = fmt.Fprintf(w, "Warning: catalog %s is not published on grc.store; skipping (the plugin must carry its own copy)\n", c)
			continue
		}
		if err != nil {
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
