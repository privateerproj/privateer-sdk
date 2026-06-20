// Package publish is the grc.store producer path: assemble a multi-platform OCI
// plugin index from a GoReleaser dist directory, push it to the hub's registry,
// keyless-sign it, and /sync so the hub ingests and verifies it. The command
// package owns the `pvtr publish` CLI wiring and calls Publish; the logic and
// its tests live here so command/ stays a thin layer over the internal packages.
package publish

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/privateerproj/privateer-sdk/internal/auth"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
	"github.com/revanite-io/grc-store-protocol/spdx"
)

// Params are the inputs to Publish.
type Params struct {
	// DistDir is the GoReleaser dist directory (artifacts.json + metadata.json).
	DistDir string
	// Registry, when set, is a registry override WITH scheme (http:// or https://)
	// that pushes to a non-hub host anonymously and skips signing + sync — a
	// testing escape hatch. Empty is the real hub publish path.
	Registry string
	// NoSync is a push-only smoke mode: skip signing AND /sync.
	NoSync bool

	// resolveManifest overrides how the plugin's publish manifest is obtained
	// from the resolved build. Nil uses execPublishManifest (select the host
	// binary and run its publish-manifest subcommand); tests inject a stub so
	// they need no real plugin binary for the host platform.
	resolveManifest func(ctx context.Context, bins []oci.PlatformBinary) (pluginkit.PublishManifest, error)
}

// Publish runs the complete producer flow. The plugin coordinate and the
// control-catalog linkage it evaluates are read from the built plugin itself
// (its publish-manifest subcommand), so the data lives in the plugin's source
// and can't be forged at publish time by someone who doesn't own the plugin.
// Progress is written to w; the caller owns flushing w.
func Publish(ctx context.Context, w io.Writer, p Params) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Load the GoReleaser build (darwin universal re-expanded to amd64+arm64).
	version, bins, err := oci.LoadGoReleaserBuild(p.DistDir)
	if err != nil {
		return fmt.Errorf("loading GoReleaser build from %s: %w", p.DistDir, err)
	}

	// 2. Verify every binary is actually a Privateer plugin (carries the
	//    go-plugin handshake marker) BEFORE running or pushing anything — catches
	//    a --dist pointed at the wrong build, or a non-plugin binary. Enforced on
	//    ALL paths incl. --registry: publishing a non-plugin is a mistake
	//    regardless of target, and the scan is cheap.
	if err := oci.ValidatePluginBinaries(bins); err != nil {
		return err
	}

	// 3. Read the publish manifest FROM THE PLUGIN ITSELF: run the host-platform
	//    binary's publish-manifest subcommand for the coordinate + evaluated
	//    catalogs. The plugin is the source of truth — the coordinate lives in its
	//    source and the evaluates linkage in its embedded catalogs — so neither is
	//    a publish-time flag a non-owner could forge.
	resolve := p.resolveManifest
	if resolve == nil {
		resolve = execPublishManifest
	}
	manifest, err := resolve(ctx, bins)
	if err != nil {
		return fmt.Errorf("reading publish manifest from the plugin: %w", err)
	}
	coordinate := strings.TrimSpace(manifest.Coordinate)
	if coordinate == "" {
		return fmt.Errorf("the plugin declared no publish coordinate — the author must set orchestrator.Publisher (coordinate = <publisher>/<plugin-name>)")
	}
	_, _ = fmt.Fprintf(w, "Loaded %s version %s (%d platforms)\n", coordinate, version, len(bins))

	// Convert the plugin-declared evaluates (the public pluginkit type) into the
	// shared protocol type at this boundary. Keeping pluginkit.EvaluatesDeclaration
	// as our own public struct insulates community plugins from grc-store-protocol's
	// v0.x churn; the fields are identical, so this is a straight copy.
	evaluates := make([]pluginspec.Evaluate, len(manifest.Evaluates))
	for i, e := range manifest.Evaluates {
		evaluates[i] = pluginspec.Evaluate{
			Catalog:        e.Catalog,
			CatalogVersion: e.CatalogVersion,
			RequirementIDs: e.RequirementIDs,
		}
	}

	assembleParams := oci.AssembleParams{
		Coordinate: coordinate,
		Plugin:     coordinate,
		Version:    version,
		License:    strings.TrimSpace(manifest.License),
		Binaries:   bins,
		Evaluates:  evaluates,
	}

	// A --registry override pushes to a non-hub host for testing; it requires an
	// explicit scheme (http:// or https://) so there is no separate --plain-http
	// flag. Parse it up front so a bad value fails before any work.
	var overrideHost string
	var overridePlainHTTP bool
	if p.Registry != "" {
		overrideHost, overridePlainHTTP, err = parseRegistryOverride(p.Registry)
		if err != nil {
			return err
		}
	}

	// SPDX gate: whenever a license is declared, validate and canonicalize it
	// BEFORE assembly so the SIGNED config always carries the canonical form, on
	// every push path (this is grcli's strict check — stricter than the hub, which
	// is lenient on unknown ids — so a typo'd or unknown id fails locally rather
	// than being signed and pushed). "Signed ⇒ canonical" is an invariant; the
	// presence REQUIREMENT, by contrast, is part of the hub contract and is
	// enforced only on the real publish path below.
	if assembleParams.License != "" {
		canonical, err := spdx.Canonicalize(assembleParams.License)
		if err != nil {
			return fmt.Errorf("plugin license %q is not a valid SPDX expression (set orchestrator.License): %w", assembleParams.License, err)
		}
		assembleParams.License = canonical
	}

	// 4. PREFLIGHT: validate the hub's required fields BEFORE any push/sign, so a
	//    malformed index never lands (and orphans signed bytes) in the registry.
	//    --registry is the anonymous smoke path and is exempt (it never syncs, so
	//    the hub contract — including the license REQUIREMENT — doesn't apply; it's
	//    for testing assembly/push only).
	if p.Registry == "" {
		// The empty case gets a clearer message than the SPDX parser would give.
		if assembleParams.License == "" {
			return fmt.Errorf("the plugin declared no license — set orchestrator.License to an SPDX expression (e.g. \"Apache-2.0\")")
		}
		if err := oci.ValidateForPublish(assembleParams); err != nil {
			return fmt.Errorf("plugin is not publishable: %w", err)
		}
	}

	// 5. Assemble the multi-platform OCI index + config/binary blobs.
	idx, err := oci.AssembleIndex(assembleParams)
	if err != nil {
		return fmt.Errorf("assembling index: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Assembled index %s (%d child manifests)\n", idx.IndexDigest(), len(idx.Manifests))

	// 6. --registry override: push to a non-hub host (a local zot / GHCR) for
	//    testing. That path is anonymous and skips auth + sync — it's the escape
	//    hatch, not the real publish.
	if p.Registry != "" {
		_, _ = fmt.Fprintf(w, "Pushing to %s (--registry override; anonymous, no sync)\n", overrideHost)
		digest, perr := oci.Push(ctx, idx, oci.PushOptions{RegistryHost: overrideHost, PlainHTTP: overridePlainHTTP})
		if perr != nil {
			return fmt.Errorf("pushing index: %w", perr)
		}
		_, _ = fmt.Fprintf(w, "Pushed %s:%s\nIndex digest: %s\n", coordinate, version, digest)
		return nil
	}

	// 7. Real publish: discover the hub's registry + OIDC coordinates, get an
	//    authenticated bearer (login store or PVTR_TOKEN), mint a push-scoped
	//    registry token, authenticated push, then sync.
	disco, err := oci.NewClient().Discover(ctx)
	if err != nil {
		return fmt.Errorf("hub discovery: %w", err)
	}
	host, err := disco.RegistryHost()
	if err != nil {
		return fmt.Errorf("resolving registry host: %w", err)
	}

	bearer, err := auth.BearerToken(ctx, disco.OIDCIssuer, disco.OIDCClientID)
	if err != nil {
		return fmt.Errorf("authentication required to publish (run `pvtr login`, or set PVTR_TOKEN in CI): %w", err)
	}
	regToken, err := oci.MintRegistryToken(ctx, oci.HubURL(), coordinate, bearer)
	if err != nil {
		return fmt.Errorf("minting registry push token: %w", err)
	}
	// Fail fast BEFORE push/sign: the hub grants pull-only (not an error) when the
	// caller doesn't own the namespace, so without this check we'd mint a
	// pull-only token, prompt for a sigstore sign-in, and only then fail at the
	// raw registry push. Detect the denied push from the minted token's scope.
	if !regToken.GrantsPush() {
		ns, pid, _ := strings.Cut(coordinate, "/")
		return fmt.Errorf("publishing to %s/%s/%s requires ownership of namespace %q — create or claim it first (e.g. at %s/%s, or POST %s/v1/orgs), then re-publish",
			ns, oci.ReservedPluginSegment, pid,
			ns, uiBaseFromHub(disco.HubURL), ns, oci.HubURL())
	}

	pushOpts := oci.PushOptions{
		RegistryHost:  host,
		PlainHTTP:     disco.PlainHTTP(),
		RegistryToken: regToken.Token,
	}

	_, _ = fmt.Fprintf(w, "Pushing to %s (hub %s)\n", host, oci.HubURL())
	digest, err := oci.Push(ctx, idx, pushOpts)
	if err != nil {
		return fmt.Errorf("pushing index: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Pushed %s:%s (index %s)\n", coordinate, version, digest)

	// --no-sync is a push-only smoke mode: skip signing AND sync, so it needs no
	//  signing identity. (A signed index that isn't synced is useless; signing is
	//  bundled with the sync that ingests it.)
	if p.NoSync {
		_, _ = fmt.Fprintf(w, "Pushed only (--no-sync): skipped signing + sync.\n")
		return nil
	}

	// 8. Sign the index against PUBLIC-GOOD Fulcio and attach the bundle as the
	//    index's OCI referrer. The signing identity is a SEPARATE token from the
	//    registry bearer above: Fulcio only trusts public OIDC issuers (GitHub
	//    Actions / the interactive sigstore login), not the grc.store Keycloak.
	//    In CI this is seamless; for a human it is a second browser sign-in.
	signTok, err := auth.SigningIDToken(ctx, w)
	if err != nil {
		return fmt.Errorf("acquiring signing identity (public-good Fulcio; distinct from `pvtr login`): %w", err)
	}
	_, _ = fmt.Fprintf(w, "Signing %s:%s (keyless, public-good Sigstore)...\n", coordinate, version)
	if err := oci.SignAndAttach(ctx, idx, pushOpts, oci.SignerOptions{IDToken: signTok}); err != nil {
		return fmt.Errorf("signing index: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Syncing %s:%s to the hub...\n", coordinate, version)
	if err := oci.Sync(ctx, oci.HubURL(), coordinate, version, bearer); err != nil {
		// The hub verifies the signature at ingest; it surfaces actionable codes
		// (plugin_signer_mismatch, registry_diverged, …) — pass them verbatim.
		return fmt.Errorf("hub sync: %w", err)
	}
	_, _ = fmt.Fprintf(w, "Published %s:%s\n", coordinate, version)
	return nil
}
