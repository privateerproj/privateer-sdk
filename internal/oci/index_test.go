package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

// writeBinary writes fake binary bytes and returns the path.
func writeBinary(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func baseParams(bins []PlatformBinary) AssembleParams {
	return AssembleParams{
		Coordinate: "ossf/pvtr-github-repo-scanner",
		Plugin:     "ossf/pvtr-github-repo-scanner",
		Version:    "1.4.0",
		License:    "Apache-2.0",
		Binaries:   bins,
		Evaluates: []pluginspec.Evaluate{
			{Catalog: "ossf/osps.baseline", CatalogVersion: "2025.02", RequirementIDs: []string{"OSPS-AC-01"}},
		},
	}
}

func TestAssembleIndex_LinuxAndDarwinUniversal(t *testing.T) {
	dir := t.TempDir()
	linux := writeBinary(t, dir, "linux-bin", []byte("linux-binary-bytes"))
	// The darwin universal fat binary: ONE file, referenced by both arches.
	fat := writeBinary(t, dir, "darwin-fat", []byte("darwin-universal-fat-bytes"))

	bins := []PlatformBinary{
		{OS: "linux", Arch: "amd64", Path: linux, Entrypoint: "github-repo"},
		{OS: "darwin", Arch: "amd64", Path: fat, Entrypoint: "github-repo"},
		{OS: "darwin", Arch: "arm64", Path: fat, Entrypoint: "github-repo"},
	}

	idx, err := AssembleIndex(baseParams(bins))
	if err != nil {
		t.Fatalf("AssembleIndex: %v", err)
	}

	// 3 platform descriptors in the index.
	var index ocispec.Index
	if err := json.Unmarshal(idx.Index.Data, &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(index.Manifests) != 3 {
		t.Fatalf("expected 3 child descriptors, got %d", len(index.Manifests))
	}

	// The two darwin children must reference the SAME binary layer digest
	// (one fat binary, two descriptors — the §3.1 contract).
	darwinLayerDigests := map[string]bool{}
	for _, child := range idx.Manifests {
		var m ocispec.Manifest
		if err := json.Unmarshal(child.Data, &m); err != nil {
			t.Fatalf("unmarshal child: %v", err)
		}
		var cfg pluginspec.Config
		// find this child's config blob and decode it
		for _, b := range idx.Blobs {
			if b.Digest == m.Config.Digest {
				if err := json.Unmarshal(b.Data, &cfg); err != nil {
					t.Fatalf("unmarshal config: %v", err)
				}
			}
		}
		if cfg.Platform.OS == "darwin" {
			darwinLayerDigests[m.Layers[0].Digest.String()] = true
		}
	}
	if len(darwinLayerDigests) != 1 {
		t.Errorf("darwin amd64/arm64 should share ONE binary layer digest, got %d distinct", len(darwinLayerDigests))
	}

	// Blob dedup: linux bin + fat bin = 2 distinct binary blobs, plus 3 config
	// blobs (each platform's config differs by os/arch) = 5 total blobs.
	binaryBlobs, configBlobs := 0, 0
	for _, b := range idx.Blobs {
		switch b.MediaType {
		case MediaTypePluginBinary:
			binaryBlobs++
		case MediaTypePluginConfig:
			configBlobs++
		}
	}
	if binaryBlobs != 2 {
		t.Errorf("expected 2 deduplicated binary blobs (linux + 1 shared darwin fat), got %d", binaryBlobs)
	}
	if configBlobs != 3 {
		t.Errorf("expected 3 config blobs (one per platform descriptor), got %d", configBlobs)
	}
}

func TestAssembleIndex_ConfigBlobContent(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("bytes"))
	bins := []PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "github-repo"}}

	idx, err := AssembleIndex(baseParams(bins))
	if err != nil {
		t.Fatalf("AssembleIndex: %v", err)
	}

	var cfg pluginspec.Config
	for _, b := range idx.Blobs {
		if b.MediaType == MediaTypePluginConfig {
			if err := json.Unmarshal(b.Data, &cfg); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
		}
	}
	if cfg.Plugin != "ossf/pvtr-github-repo-scanner" {
		t.Errorf("plugin = %q", cfg.Plugin)
	}
	if cfg.Version != "1.4.0" {
		t.Errorf("version = %q", cfg.Version)
	}
	if cfg.License != "Apache-2.0" {
		t.Errorf("license = %q (must be written into the signed config blob)", cfg.License)
	}
	if cfg.Entrypoint != "github-repo" {
		t.Errorf("entrypoint = %q (must be the go-plugin name, not the repo)", cfg.Entrypoint)
	}
	if cfg.Protocol != "netrpc" {
		t.Errorf("protocol = %q", cfg.Protocol)
	}
	if cfg.Platform.OS != "linux" || cfg.Platform.Arch != "amd64" {
		t.Errorf("platform = %+v", cfg.Platform)
	}
	if len(cfg.Evaluates) != 1 || cfg.Evaluates[0].Catalog != "ossf/osps.baseline" {
		t.Errorf("evaluates = %+v", cfg.Evaluates)
	}
}

func TestAssembleIndex_Deterministic(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("bytes"))
	bins := []PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}

	a, err := AssembleIndex(baseParams(bins))
	if err != nil {
		t.Fatal(err)
	}
	b, err := AssembleIndex(baseParams(bins))
	if err != nil {
		t.Fatal(err)
	}
	if a.IndexDigest() != b.IndexDigest() {
		t.Errorf("index digest not deterministic: %s vs %s", a.IndexDigest(), b.IndexDigest())
	}
}

func TestAssembleIndex_Validation(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("x"))
	ok := []PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}

	cases := []struct {
		name string
		p    AssembleParams
	}{
		{"no coordinate", AssembleParams{Plugin: "o/r", Version: "1", Binaries: ok}},
		{"no plugin", AssembleParams{Coordinate: "n/i", Version: "1", Binaries: ok}},
		{"no version", AssembleParams{Coordinate: "n/i", Plugin: "o/r", Binaries: ok}},
		{"no binaries", AssembleParams{Coordinate: "n/i", Plugin: "o/r", Version: "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AssembleIndex(tc.p); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestAssembleIndex_MissingBinaryFileErrors(t *testing.T) {
	bins := []PlatformBinary{{OS: "linux", Arch: "amd64", Path: "/nonexistent/binary", Entrypoint: "e"}}
	if _, err := AssembleIndex(baseParams(bins)); err == nil {
		t.Fatal("expected error reading missing binary, got nil")
	}
}

func TestHostPlatformBinary(t *testing.T) {
	bins := []PlatformBinary{
		{OS: "plan9", Arch: "mips", Path: "/x", Entrypoint: "e"},
	}
	if _, err := HostPlatformBinary(bins); err == nil {
		t.Fatal("expected error when host platform absent")
	}
}

// Guard: child manifests and index carry schemaVersion 2.
func TestAssembleIndex_SchemaVersion(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("x"))
	idx, err := AssembleIndex(baseParams([]PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}))
	if err != nil {
		t.Fatal(err)
	}
	var v specs.Versioned
	if err := json.Unmarshal(idx.Index.Data, &v); err != nil {
		t.Fatal(err)
	}
	if v.SchemaVersion != 2 {
		t.Errorf("index schemaVersion = %d, want 2", v.SchemaVersion)
	}
}

// --- Maintainer hard-requirement conformance (each is enforced by the hub
// verifier; a regression here is a 422 at sync) ---

// HR: the tag must resolve to an OCI image INDEX even for a single platform.
func TestConformance_SinglePlatformStillIndex(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("only-linux"))
	idx, err := AssembleIndex(baseParams([]PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}))
	if err != nil {
		t.Fatal(err)
	}
	if idx.Index.MediaType != ocispec.MediaTypeImageIndex {
		t.Errorf("top-level mediaType = %q, want image index", idx.Index.MediaType)
	}
	var index ocispec.Index
	if err := json.Unmarshal(idx.Index.Data, &index); err != nil {
		t.Fatal(err)
	}
	if index.MediaType != ocispec.MediaTypeImageIndex {
		t.Errorf("index.mediaType = %q", index.MediaType)
	}
	if len(index.Manifests) != 1 {
		t.Errorf("expected 1 child, got %d", len(index.Manifests))
	}
}

// HR: each child has config mediaType EXACTLY the plugin config type and EXACTLY
// one binary layer of the plugin binary type; and the index child descriptor
// carries the platform.
func TestConformance_ChildMediaTypesAndPlatform(t *testing.T) {
	dir := t.TempDir()
	linux := writeBinary(t, dir, "l", []byte("l"))
	fat := writeBinary(t, dir, "fat", []byte("fat"))
	idx, err := AssembleIndex(baseParams([]PlatformBinary{
		{OS: "linux", Arch: "amd64", Path: linux, Entrypoint: "e"},
		{OS: "darwin", Arch: "amd64", Path: fat, Entrypoint: "e"},
		{OS: "darwin", Arch: "arm64", Path: fat, Entrypoint: "e"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(idx.Index.Data, &index); err != nil {
		t.Fatal(err)
	}
	// Every index child descriptor must declare its platform (the hub prefers
	// this over the config-declared platform).
	for _, d := range index.Manifests {
		if d.Platform == nil || d.Platform.OS == "" || d.Platform.Architecture == "" {
			t.Errorf("child descriptor missing platform: %+v", d.Platform)
		}
	}
	// Both darwin descriptors must be present with distinct arch on the
	// descriptor even though they share a layer blob.
	darwinArches := map[string]bool{}
	for _, d := range index.Manifests {
		if d.Platform != nil && d.Platform.OS == "darwin" {
			darwinArches[d.Platform.Architecture] = true
		}
	}
	if !darwinArches["amd64"] || !darwinArches["arm64"] {
		t.Errorf("darwin descriptors should be amd64+arm64, got %v", darwinArches)
	}

	// Each child manifest: exactly one binary layer of the right type, and the
	// config media type exactly the plugin config type.
	for _, mb := range idx.Manifests {
		var m ocispec.Manifest
		if err := json.Unmarshal(mb.Data, &m); err != nil {
			t.Fatal(err)
		}
		if m.Config.MediaType != MediaTypePluginConfig {
			t.Errorf("config mediaType = %q, want %q", m.Config.MediaType, MediaTypePluginConfig)
		}
		if len(m.Layers) != 1 {
			t.Fatalf("expected exactly 1 layer, got %d", len(m.Layers))
		}
		if m.Layers[0].MediaType != MediaTypePluginBinary {
			t.Errorf("layer mediaType = %q, want %q", m.Layers[0].MediaType, MediaTypePluginBinary)
		}
	}
}

// HR: entrypoint, version, plugin, and the full evaluates list are byte-identical
// across every child, and evaluates is in a deterministic (sorted) order even
// when the caller passes it unsorted.
func TestConformance_ConfigByteIdenticalAcrossChildrenAndDeterministicEvaluates(t *testing.T) {
	dir := t.TempDir()
	a := writeBinary(t, dir, "a", []byte("a"))
	b := writeBinary(t, dir, "b", []byte("b"))

	// Entrypoint is GoReleaser's extra.Binary, which is byte-identical across
	// platforms (no ".exe" — the real fixture confirms windows extra.Binary is
	// "github-repo", only the artifact NAME carries .exe). The hub requires
	// entrypoint to match across children, so the loader must never .exe-suffix
	// the entrypoint.
	p := baseParams([]PlatformBinary{
		{OS: "linux", Arch: "amd64", Path: a, Entrypoint: "github-repo"},
		{OS: "windows", Arch: "amd64", Path: b, Entrypoint: "github-repo"},
	})
	// Deliberately UNSORTED evaluates with unsorted requirement_ids to prove
	// canonicalization makes children agree.
	p.Evaluates = []pluginspec.Evaluate{
		{Catalog: "z/last", CatalogVersion: "2025.01", RequirementIDs: []string{"R2", "R1"}},
		{Catalog: "a/first", CatalogVersion: "2025.02", RequirementIDs: []string{"Z", "A"}},
	}

	idx, err := AssembleIndex(p)
	if err != nil {
		t.Fatal(err)
	}

	// Collect each child's config (minus the platform field, which is the only
	// field that legitimately differs).
	type stable struct {
		Plugin     string
		Version    string
		Entrypoint string
		Protocol   string
		Evaluates  []pluginspec.Evaluate
	}
	var seen []stable
	for _, blob := range idx.Blobs {
		if blob.MediaType != MediaTypePluginConfig {
			continue
		}
		var cfg pluginspec.Config
		if err := json.Unmarshal(blob.Data, &cfg); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, stable{cfg.Plugin, cfg.Version, cfg.Entrypoint, cfg.Protocol, cfg.Evaluates})
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 config blobs, got %d", len(seen))
	}
	// Byte-identical (excluding platform): re-marshal and compare.
	j0, _ := json.Marshal(seen[0])
	j1, _ := json.Marshal(seen[1])
	if string(j0) != string(j1) {
		t.Errorf("config (minus platform) differs across children:\n%s\n%s", j0, j1)
	}
	// Evaluates sorted by catalog: "a/first" before "z/last".
	ev := seen[0].Evaluates
	if len(ev) != 2 || ev[0].Catalog != "a/first" || ev[1].Catalog != "z/last" {
		t.Errorf("evaluates not sorted by catalog: %+v", ev)
	}
	// requirement_ids sorted within an entry.
	if len(ev[0].RequirementIDs) != 2 || ev[0].RequirementIDs[0] != "A" || ev[0].RequirementIDs[1] != "Z" {
		t.Errorf("requirement_ids not sorted: %v", ev[0].RequirementIDs)
	}
}

// HR: config.version must equal the tag the index is pushed under.
func TestConformance_ConfigVersionEqualsTag(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("x"))
	idx, err := AssembleIndex(baseParams([]PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, blob := range idx.Blobs {
		if blob.MediaType != MediaTypePluginConfig {
			continue
		}
		var cfg pluginspec.Config
		if err := json.Unmarshal(blob.Data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Version != idx.Version {
			t.Errorf("config.version %q != index tag %q", cfg.Version, idx.Version)
		}
	}
}

// HR: evaluates is encoded FLAT with a combined "catalog: <ns>/<id>" coordinate
// (not pre-split into namespace/id) in the signed config blob.
func TestConformance_EvaluatesCombinedCatalogNotPreSplit(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "b", []byte("x"))
	idx, err := AssembleIndex(baseParams([]PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "e"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, blob := range idx.Blobs {
		if blob.MediaType != MediaTypePluginConfig {
			continue
		}
		// Decode into a generic map to assert the wire shape: a single "catalog"
		// string, and NO "catalog_namespace"/"catalog_id" split keys.
		var raw map[string]any
		if err := json.Unmarshal(blob.Data, &raw); err != nil {
			t.Fatal(err)
		}
		evs, ok := raw["evaluates"].([]any)
		if !ok || len(evs) == 0 {
			t.Fatal("evaluates missing")
		}
		e0 := evs[0].(map[string]any)
		if _, ok := e0["catalog"].(string); !ok {
			t.Errorf("evaluates[0] has no combined string 'catalog' field: %v", e0)
		}
		if e0["catalog"].(string) != "ossf/osps.baseline" {
			t.Errorf("catalog = %v, want combined ns/id", e0["catalog"])
		}
		for _, forbidden := range []string{"catalog_namespace", "catalog_id"} {
			if _, present := e0[forbidden]; present {
				t.Errorf("config blob must NOT pre-split the catalog coordinate; found %q", forbidden)
			}
		}
	}
}
