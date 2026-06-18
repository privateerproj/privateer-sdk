package oci

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/revanite-io/grc-store-protocol/mediatype"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

// grc.store plugin media types — re-exported from the shared protocol contract
// so the wire strings have a single source of truth (grc-store-protocol/mediatype,
// ADR-0035) instead of a hand-synced copy.
const (
	MediaTypePluginConfig = mediatype.PluginConfig
	MediaTypePluginBinary = mediatype.PluginBinary
)

// defaultProtocol is the go-plugin transport. netrpc matches the SDK's
// hashicorp/go-plugin usage; surfaced in the config blob for the installer.
const defaultProtocol = "netrpc"

// The signed config-blob schema (vnd.grc-store.plugin.config.v1+json) — the
// descriptor a bare binary can't self-carry (entrypoint rename, protocol,
// in-model `evaluates` linkage) — is the shared producer↔hub contract type
// pluginspec.Config (ADR-0035). pvtr writes and signs it; the hub reads it as
// the authoritative source on sync. We use pluginspec.Config/Platform/Evaluate
// directly so the wire shape can't drift from the hub's reader.

// blob is an assembled, content-addressed in-memory artifact: its bytes, media
// type, and digest. The push layer (oras) writes these to a registry; keeping
// assembly in-memory and digest-pure makes it unit-testable with no network.
type blob struct {
	MediaType string
	Data      []byte
	Digest    digest.Digest
}

func newBlob(mediaType string, data []byte) blob {
	return blob{
		MediaType: mediaType,
		Data:      data,
		Digest:    digest.FromBytes(data),
	}
}

func (b blob) descriptor() ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: b.MediaType,
		Digest:    b.Digest,
		Size:      int64(len(b.Data)),
	}
}

// Descriptor returns the OCI descriptor for this blob. Exported so callers with
// an AssembledIndex (e.g. the verify package's tests) can reference the index
// descriptor without the push layer.
func (b blob) Descriptor() ocispec.Descriptor { return b.descriptor() }

// AssembledIndex is the full set of content-addressed artifacts for a plugin
// version: the image index, every child manifest, and every config + binary
// blob. The push layer walks Blobs (leaves first) then Manifests then Index.
type AssembledIndex struct {
	Coordinate string // "<namespace>/<plugin_id>"
	Version    string
	Index      blob   // the OCI image index (the digest the signature covers)
	Manifests  []blob // child image manifests, one per platform descriptor
	Blobs      []blob // config + binary blobs (deduplicated by digest)
}

// IndexDigest returns the assembled index's digest (sha256:...), the value the
// signature is over and that the manifest records.
func (idx *AssembledIndex) IndexDigest() string { return idx.Index.Digest.String() }

// AssembleParams are the inputs to AssembleIndex.
type AssembleParams struct {
	// Coordinate is "<namespace>/<plugin_id>" — the grc.store push coordinate.
	Coordinate string
	// Plugin is the "owner/repo" recorded in each config blob's "plugin" field.
	Plugin string
	// Version is the release version (tag without leading v is fine; recorded
	// verbatim in the config blobs).
	Version string
	// License is the publication license as a (canonical) SPDX expression,
	// written into every config blob. grc.store requires it; the caller
	// (pvtr publish) validates and canonicalizes it before assembly.
	License string
	// Binaries are the resolved per-platform binaries (darwin universal already
	// re-expanded by LoadGoReleaserBuild).
	Binaries []PlatformBinary
	// Evaluates is the control-catalog linkage, identical across platforms,
	// written into every config blob. Optional.
	Evaluates []pluginspec.Evaluate
}

// AssembleIndex builds the full OCI image index for a plugin version from the
// resolved per-platform binaries. It is pure (reads the binary files, produces
// content-addressed blobs in memory) and does no network or signing — signing
// is cosign's job after this returns, push is oras's. Binary blobs are
// deduplicated by digest, so the two darwin descriptors over one fat binary
// share a single layer blob (the §3.1 contract).
func AssembleIndex(p AssembleParams) (*AssembledIndex, error) {
	if p.Coordinate == "" {
		return nil, fmt.Errorf("coordinate is required")
	}
	if p.Plugin == "" {
		return nil, fmt.Errorf("plugin (owner/repo) is required")
	}
	if p.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if len(p.Binaries) == 0 {
		return nil, fmt.Errorf("no binaries to assemble")
	}

	// Canonicalize `evaluates` ONCE so every child carries a byte-identical,
	// deterministically-ordered list. The hub compares evaluates across children
	// order-sensitively, so a stable order is a hard requirement, not a nicety.
	evaluates := canonicalEvaluates(p.Evaluates)

	out := &AssembledIndex{Coordinate: p.Coordinate, Version: p.Version}
	blobsByDigest := map[digest.Digest]bool{}
	// binBlobsByPath memoizes binary reads: the darwin universal fat binary is
	// referenced by two PlatformBinary entries (amd64 + arm64) with the same
	// Path — memoizing avoids reading and SHA-256ing the same file twice.
	binBlobsByPath := map[string]blob{}
	var manifestDescriptors []ocispec.Descriptor

	for _, b := range p.Binaries {
		binBlob, ok := binBlobsByPath[b.Path]
		if !ok {
			binData, err := os.ReadFile(b.Path)
			if err != nil {
				return nil, fmt.Errorf("reading binary for %s/%s at %s: %w", b.OS, b.Arch, b.Path, err)
			}
			binBlob = newBlob(MediaTypePluginBinary, binData)
			binBlobsByPath[b.Path] = binBlob
		}

		cfg := pluginspec.Config{
			Plugin:     p.Plugin,
			Version:    p.Version,
			License:    p.License,
			Platform:   pluginspec.Platform{OS: b.OS, Arch: b.Arch},
			Entrypoint: b.Entrypoint,
			Protocol:   defaultProtocol,
			Evaluates:  evaluates,
		}
		cfgData, err := json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshalling config blob for %s/%s: %w", b.OS, b.Arch, err)
		}
		cfgBlob := newBlob(MediaTypePluginConfig, cfgData)

		// Deduplicate blobs by digest: the universal darwin binary is shared by
		// two platform descriptors but stored once.
		for _, bl := range []blob{cfgBlob, binBlob} {
			if !blobsByDigest[bl.Digest] {
				blobsByDigest[bl.Digest] = true
				out.Blobs = append(out.Blobs, bl)
			}
		}

		manifest := ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    cfgBlob.descriptor(),
			Layers:    []ocispec.Descriptor{binBlob.descriptor()},
		}
		manData, err := json.Marshal(manifest)
		if err != nil {
			return nil, fmt.Errorf("marshalling child manifest for %s/%s: %w", b.OS, b.Arch, err)
		}
		manBlob := newBlob(ocispec.MediaTypeImageManifest, manData)
		out.Manifests = append(out.Manifests, manBlob)

		desc := manBlob.descriptor()
		desc.Platform = &ocispec.Platform{OS: b.OS, Architecture: b.Arch}
		manifestDescriptors = append(manifestDescriptors, desc)
	}

	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: manifestDescriptors,
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("marshalling image index: %w", err)
	}
	out.Index = newBlob(ocispec.MediaTypeImageIndex, indexData)
	return out, nil
}

// canonicalEvaluates returns a deterministically-ordered deep copy of the
// evaluates list: entries sorted by (catalog, catalog_version), and each entry's
// requirement_ids sorted. The order MUST be stable across children — the hub
// compares the list byte-identically and order-sensitively, so a non-deterministic
// order (e.g. a map-derived caller) would make children mismatch. Returns nil for
// an empty input; valid publishes always carry non-empty evaluates
// (ValidateForPublish gates empty), so the nil case never reaches a published blob.
func canonicalEvaluates(in []pluginspec.Evaluate) []pluginspec.Evaluate {
	if len(in) == 0 {
		return nil
	}
	out := make([]pluginspec.Evaluate, len(in))
	for i, e := range in {
		reqs := slices.Clone(e.RequirementIDs)
		slices.Sort(reqs)
		out[i] = pluginspec.Evaluate{
			Catalog:        e.Catalog,
			CatalogVersion: e.CatalogVersion,
			RequirementIDs: reqs,
		}
	}
	slices.SortStableFunc(out, func(a, b pluginspec.Evaluate) int {
		return cmp.Or(cmp.Compare(a.Catalog, b.Catalog), cmp.Compare(a.CatalogVersion, b.CatalogVersion))
	})
	return out
}

// HostPlatformBinary returns the PlatformBinary matching the running host's
// os/arch, or an error naming the available platforms. Used by the local push
// smoke path and as the analogue of the installer's child-selection step.
func HostPlatformBinary(bins []PlatformBinary) (PlatformBinary, error) {
	for _, b := range bins {
		if b.OS == runtime.GOOS && b.Arch == runtime.GOARCH {
			return b, nil
		}
	}
	var avail []string
	for _, b := range bins {
		avail = append(avail, b.OS+"/"+b.Arch)
	}
	return PlatformBinary{}, fmt.Errorf("no binary for host %s/%s (have: %s)", runtime.GOOS, runtime.GOARCH, strings.Join(avail, ", "))
}
