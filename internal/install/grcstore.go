package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/utils"
)

// FromStore resolves a plugin DIRECTLY against grc.store (the single source of
// truth): parse the <ns>/<id>[@<version>] coordinate, confirm it exists via GET
// /v1/plugins/<ns>/<id>, resolve the version, then pull + verify + install. No
// legacy plugin-data registry, no GitHub fallback. Progress is written to w; the
// caller owns flushing w.
func FromStore(ctx context.Context, w io.Writer, arg string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	namespace, pluginId, requestedVersion, err := parseCoordinate(arg)
	if err != nil {
		return err
	}

	hub := oci.NewClient()
	_, _ = fmt.Fprintf(w, "Resolving %s/%s on grc.store (%s)...\n", namespace, pluginId, oci.HubURL())

	pluginDetails, err := hub.GetPluginDetails(ctx, namespace, pluginId)
	if err != nil {
		// This is when we know it is not found on GRC store, or otherwise there was a critical error querying
		return fmt.Errorf("resolution: %w", err)
	}

	release, err := pluginDetails.ResolveRelease(requestedVersion)
	if err != nil {
		return err
	}

	return pullVerifyInstall(ctx, w, hub, pluginDetails, release)
}

// pullVerifyInstall runs the verified install core: pull the signed index,
// verify it end-to-end (§6: keyless signature + camp-(b) TOFU identity + full
// digest walk), and only then write the verified binary. It FAILS CLOSED on any
// verification error — it never falls back to an unverified copy.
//
// The hub client is passed in (rather than created fresh) so discovery uses
// the same authenticated client as the plugin-detail lookup above — one
// client for both hub calls.
func pullVerifyInstall(ctx context.Context, w io.Writer, hub *oci.Client, detail *oci.PluginDetail, release *oci.PluginRelease) error {
	coordinate := detail.Coordinate()

	fetchedIndex, err := fetchIndex(ctx, w, hub, release, coordinate)
	if err != nil {
		return err
	}

	// Cross-check the pulled index digest against the hub-recorded digest.
	if release.IndexDigest != "" {
		if fetchedIndex.IndexDescriptor.Digest.String() != release.IndexDigest {
			return fmt.Errorf("registry diverged from hub for %s:%s: registry index digest %s != hub-recorded %s — refusing to install",
				coordinate, release.Version, fetchedIndex.IndexDescriptor.Digest, release.IndexDigest)
		}
	} else {
		_, _ = fmt.Fprintf(w, "Warning: hub recorded no index digest for %s:%s; skipping registry-divergence cross-check\n", coordinate, release.Version)
	}

	// Load the manifest first to read any previously-pinned signer identity
	// (camp (b) TOFU): empty on first install, enforced on update.
	destDir := config.GetBinariesPath()
	m, err := manifest.Load(destDir)
	if err != nil {
		return fmt.Errorf("loading plugin manifest: %w", err)
	}

	// Pin-precedence for the identity policy:
	//   1. Local manifest pin (if set) — enforces the TOFU pin established at
	//      first install and protects against a compromised hub changing the
	//      signer identity field.
	//   2. Hub's authoritative signer_identity — used on first install so the
	//      hub's known-good identity seeds the local TOFU pin, rather than
	//      accepting any valid keyless identity blindly.
	//   3. Empty (open TOFU) — only when both are absent (no local pin and hub
	//      has no declared identity).
	// When a local pin and a hub identity are both present but differ, we warn
	// (the publisher may have legitimately rotated identity and updated the hub)
	// but still enforce the local pin — the user must explicitly uninstall to
	// accept a new identity.
	pin, warn := pinnedIdentityFor(m.Find(coordinate), detail.SignerIdentity)
	if warn != "" {
		_, _ = fmt.Fprintf(w, "Warning: %s\n", warn)
	}
	policy := verify.IdentityPolicy{PinnedIdentity: pin}

	verifier, err := verify.NewVerifier()
	if err != nil {
		return fmt.Errorf("initializing verifier: %w", err)
	}
	verified, err := verifier.Index(ctx, fetchedIndex, policy)
	if err != nil {
		// Fail closed: surface the coordinate, never degrade to an unverified
		// install. ErrIdentityMismatch already embeds the got/pinned identities,
		// so no need to repeat the signer in this wrapper.
		return fmt.Errorf("verifying %s:%s: %w", coordinate, release.Version, err)
	}

	// Write the VERIFIED bytes under <coordinate>/<version>/<entrypoint> so that
	// multiple installed versions of the same plugin coexist on disk rather than
	// overwriting one another. The run-time resolver maps (name, version) to this
	// path via the manifest, so the entrypoint filename no longer has to be
	// globally unique.
	binaryName := verified.Entrypoint
	if runtime.GOOS == "windows" && !strings.HasSuffix(binaryName, ".exe") {
		binaryName = binaryName + ".exe"
	}
	if !validNameSegmentRegex.MatchString(binaryName) {
		return fmt.Errorf("invalid entrypoint name %q from verified config", binaryName)
	}
	// The version becomes a directory name, so reject anything that could escape
	// the binaries dir. We don't apply validNameSegmentRegex here because valid
	// semver build metadata ("1.4.0+build") would fail it; a path-separator check
	// is enough to keep the write inside the per-plugin tree.
	if verified.Version == "" || verified.Version == "." || verified.Version == ".." ||
		strings.ContainsAny(verified.Version, `/\`) {
		return fmt.Errorf("invalid plugin version %q from verified config", verified.Version)
	}

	relPath := filepath.Join(coordinate, verified.Version, binaryName)
	dest := filepath.Join(destDir, relPath)
	if !strings.HasPrefix(filepath.Clean(dest)+string(filepath.Separator), filepath.Clean(destDir)+string(filepath.Separator)) {
		return fmt.Errorf("resolved install path %q escapes binaries directory %q", dest, destDir)
	}
	if err := writeVerifiedBinary(destDir, relPath, verified.Binary); err != nil {
		return fmt.Errorf("writing plugin binary: %w", err)
	}

	// Record provenance for update/re-verify + TOFU (pin on first install).
	// Route the write through manifest.Update so it re-reads under a lock before
	// adding: concurrent installs of other plugins (autoinstall / `install
	// --from-config`) can't clobber this entry.
	if err := manifest.Update(destDir, func(m *manifest.Manifest) error {
		m.Add(manifest.Plugin{
			Name:           coordinate,
			Version:        verified.Version,
			BinaryPath:     relPath,
			Coordinate:     coordinate,
			IndexDigest:    verified.IndexDigest,
			SignerIdentity: verified.SignerIdentity,
		})
		return nil
	}); err != nil {
		return fmt.Errorf("saving plugin manifest: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Successfully installed %s:%s (signed by %s)\n", coordinate, verified.Version, verified.SignerIdentity)
	return nil
}

func fetchIndex(ctx context.Context, w io.Writer, hub *oci.Client, release *oci.PluginRelease, coordinate string) (index *oci.FetchedIndex, err error) {
	remote, err := hub.Discover(ctx)
	if err != nil {
		err = fmt.Errorf("hub discovery: %w", err)
		return
	}
	host, err := remote.RegistryHost()
	if err != nil {
		err = fmt.Errorf("resolving registry host: %w", err)
		return
	}

	_, _ = fmt.Fprintf(w, "Pulling %s:%s from %s...\n", coordinate, release.Version, host)
	index, err = oci.PullIndex(ctx, coordinate, release.Version, oci.PullOptions{
		RegistryHost: host,
		PlainHTTP:    remote.PlainHTTP(),
	})
	if err != nil {
		err = fmt.Errorf("pulling index: %w", err)
	}
	return
}

// pinnedIdentityFor determines the effective PinnedIdentity to enforce and
// any warning to surface, applying the three-tier precedence:
//
//  1. Local manifest pin (non-empty existing.SignerIdentity) — always wins.
//  2. Hub-declared signer identity (hubIdentity) — used on first install.
//  3. Open TOFU (empty) — only when both are absent.
//
// When a local pin and hubIdentity are both non-empty and differ, a warning
// message is returned so the caller can inform the user; the local pin is
// still enforced.
func pinnedIdentityFor(existing *manifest.Plugin, hubIdentity string) (pin string, warn string) {
	if existing != nil && existing.SignerIdentity != "" {
		localPin := existing.SignerIdentity
		if hubIdentity != "" && hubIdentity != localPin {
			warn = fmt.Sprintf(
				"local signer pin %q differs from hub-declared identity %q — "+
					"enforcing local pin; uninstall first to accept the new identity",
				localPin, hubIdentity,
			)
		}
		return localPin, warn
	}
	// No local pin: seed from hub identity (may be empty → open TOFU).
	return hubIdentity, ""
}

// writeVerifiedBinary writes the verified bytes to {destDir}/{relPath}, +x,
// atomically (temp + rename) so a crash mid-write can't leave a partial binary
// that go-plugin would try to exec. relPath may contain subdirectories (the
// per-plugin, per-version layout), so the full parent chain is created. The
// atomic write itself is delegated to utils.WriteFileAtomic.
//
// A binary overwrite check is no longer needed here: each (plugin, version)
// install lands at its own coordinate/version/entrypoint path, so distinct
// plugins and versions can never resolve to the same file, and the run-time
// resolver picks the binary by name+version from the manifest rather than by a
// globally-unique filename.
func writeVerifiedBinary(destDir, relPath string, data []byte) error {
	dest := filepath.Join(destDir, relPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating binaries dir: %w", err)
	}
	return utils.WriteFileAtomic(dest, data, 0o755)
}
