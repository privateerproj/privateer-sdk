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
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Source fetches and verifies one catalog, returning it with Catalog parsed.
// *Fetcher is the real one; tests supply a stub, so install and publish can run
// without a hub or a signature.
type Source interface {
	Fetch(ctx context.Context, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error)
}

// Fetcher pulls catalogs from the hub's registry and verifies them. One
// Fetcher serves a whole run, so the sigstore trust root is parsed once however
// many catalogs a plugin declares.
type Fetcher struct {
	w           io.Writer
	hub         *oci.Client
	verifier    *verify.Verifier
	binariesDir string
}

// NewFetcher returns a Fetcher that writes warnings to w. Pass the verifier the
// caller already built, if any; with nil, one is built on the first catalog
// that reaches verification.
func NewFetcher(w io.Writer, hub *oci.Client, verifier *verify.Verifier) *Fetcher {
	return &Fetcher{w: w, hub: hub, verifier: verifier}
}

// WithBinariesDir overrides the binaries directory used for local catalog pinning.
func (f *Fetcher) WithBinariesDir(dir string) *Fetcher {
	f.binariesDir = dir
	return f
}

func (f *Fetcher) binariesPath() string {
	if f.binariesDir != "" {
		return f.binariesDir
	}
	return config.GetBinariesPath()
}

func (f *Fetcher) pinnedIdentity(c pluginkit.CatalogCoordinate, hubIdentity string) (pin, warn string) {
	localPin := readCatalogPin(pluginkit.CatalogPinPath(f.binariesPath(), c))
	return verify.PinnedIdentityFor(localPin, hubIdentity, "delete the cached catalog to accept the new identity")
}

// Fetch pulls <namespace>/<id>:<version> from the hub's registry and verifies
// it end-to-end: keyless signature against the pinned trust root, signer
// identity against the one the hub verified at ingest, and the manifest digest
// against the hub's record. It fails closed; a catalog the hub does not know is
// ErrCatalogNotFound.
func (f *Fetcher) Fetch(ctx context.Context, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	// The version is the whole of a coordinate's meaning here: it picks the OCI
	// tag to pull and the cache file to write. Rejecting an empty one at this
	// single choke point means no path can quietly resolve "whatever is latest
	// today" into the cache a plugin pinned to an exact version will read.
	if c.Version == "" {
		return nil, fmt.Errorf("catalog %s has no version; name it as <namespace>/<id>@<version>", c.Repository())
	}

	detail, err := f.hub.GetCatalogDetails(ctx, c.Namespace, c.ID)
	if err != nil {
		return nil, err
	}
	release, err := detail.Release(c.Version)
	if err != nil {
		return nil, err
	}

	host, plainHTTP, err := f.hub.Registry(ctx)
	if err != nil {
		return nil, err
	}
	fetched, err := oci.PullManifest(ctx, c.Repository(), release.Version, oci.PullOptions{RegistryHost: host, PlainHTTP: plainHTTP})
	if err != nil {
		return nil, fmt.Errorf("pulling catalog: %w", err)
	}
	if err := checkHubDigest(f.w, c, release.ManifestDigest, fetched.IndexDescriptor.Digest.String()); err != nil {
		return nil, err
	}

	pinPath := pluginkit.CatalogPinPath(f.binariesPath(), c)
	localPin := readCatalogPin(pinPath)
	pin, warn := verify.PinnedIdentityFor(localPin, detail.SignerIdentity, "delete the cached catalog to accept the new identity")
	if warn != "" {
		_, _ = fmt.Fprintf(f.w, "Warning: %s\n", warn)
	}

	if f.verifier == nil {
		if f.verifier, err = verify.NewVerifier(); err != nil {
			return nil, fmt.Errorf("initializing verifier: %w", err)
		}
	}
	verified, err := f.verifier.Catalog(ctx, fetched, verify.IdentityPolicy{PinnedIdentity: pin})
	if err != nil {
		return nil, fmt.Errorf("verifying %s: %w", c, err)
	}

	if localPin == "" && verified.SignerIdentity != "" {
		if err := writeCatalogPin(pinPath, verified.SignerIdentity); err != nil {
			return nil, fmt.Errorf("saving catalog signer pin: %w", err)
		}
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

// Install caches every coordinate under binariesDir (see
// pluginkit.CatalogCachePath), fetching and verifying the ones not already
// there; a published version is immutable, so a cached file is never
// re-fetched. A catalog src reports as not published is collected into
// unpublished (each error wraps oci.ErrCatalogNotFound and carries the hub's
// detail) while the rest are cached; what a missing catalog means is the
// caller's call. Any other failure aborts. Progress is written to w.
func Install(ctx context.Context, w io.Writer, src Source, binariesDir string, coords []pluginkit.CatalogCoordinate) (unpublished []error, err error) {
	if f, ok := src.(*Fetcher); ok && f.binariesDir == "" {
		f.binariesDir = binariesDir
	}
	for _, c := range coords {
		path := pluginkit.CatalogCachePath(binariesDir, c)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		verified, err := src.Fetch(ctx, c)
		if errors.Is(err, oci.ErrCatalogNotFound) {
			// The hub's 404 names no version, so say which coordinate was asked for.
			unpublished = append(unpublished, fmt.Errorf("%s: %w", c, err))
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("creating catalog cache dir: %w", err)
		}
		if err := utils.WriteFileAtomic(path, verified.YAML, 0o644); err != nil {
			return nil, fmt.Errorf("writing catalog %s: %w", c, err)
		}
		pinPath := pluginkit.CatalogPinPath(binariesDir, c)
		if readCatalogPin(pinPath) == "" && verified.SignerIdentity != "" {
			_ = writeCatalogPin(pinPath, verified.SignerIdentity)
		}
		_, _ = fmt.Fprintf(w, "Installed catalog %s (signed by %s)\n", c, verified.SignerIdentity)
	}
	return unpublished, nil
}

func readCatalogPin(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCatalogPin(path, identity string) error {
	if identity == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating catalog dir for signer pin: %w", err)
	}
	return utils.WriteFileAtomic(path, []byte(strings.TrimSpace(identity)+"\n"), 0o644)
}
