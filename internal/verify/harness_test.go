package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	sgverify "github.com/sigstore/sigstore-go/pkg/verify"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

const (
	testIssuer  = "https://token.actions.githubusercontent.com"
	testSANRef  = "https://github.com/ossf/pvtr-github-repo-scanner/.github/workflows/release.yml@refs/tags/v1.4.0"
	testSANBase = "https://github.com/ossf/pvtr-github-repo-scanner/.github/workflows/release.yml"
)

// testVerifier builds a Verifier whose trust root IS the virtual sigstore (so
// verification is fully offline) with sctThreshold=0 — VirtualSigstore certs
// carry no embedded SCT (production passes 1, mirroring the hub's tests).
func testVerifier(t *testing.T, vs *ca.VirtualSigstore) *Verifier {
	t.Helper()
	v, err := newVerifier(vs, 0)
	if err != nil {
		t.Fatalf("newVerifier: %v", err)
	}
	return v
}

// builtIndex is a conformant index assembled for tests, pushed into an in-memory
// oras store, ready to be fetched + walked.
type builtIndex struct {
	store    *memory.Store
	idxDesc  ocispec.Descriptor
	idxBytes []byte
	version  string
	coord    string
}

// buildHostIndex assembles a conformant index with a child for the host os/arch
// (so the happy-path walk selects it) plus a second platform, and pushes it all
// to a memory store.
func buildHostIndex(t *testing.T) *builtIndex {
	t.Helper()
	dir := t.TempDir()
	hostBin := writeFile(t, dir, "host-bin", []byte("host-binary-bytes"))
	otherBin := writeFile(t, dir, "other-bin", []byte("other-platform-bytes"))
	return assembleAndPush(t, []oci.PlatformBinary{
		{OS: runtime.GOOS, Arch: runtime.GOARCH, Path: hostBin, Entrypoint: "github-repo"},
		{OS: "plan9", Arch: "mips", Path: otherBin, Entrypoint: "github-repo"},
	})
}

// buildIndexWithoutHost assembles a conformant index that has NO child for the
// host platform (for the platform-unavailable test).
func buildIndexWithoutHost(t *testing.T) *builtIndex {
	t.Helper()
	dir := t.TempDir()
	bin := writeFile(t, dir, "bin", []byte("non-host-bytes"))
	// Pick os/arch guaranteed not to be the host.
	os1, arch1 := "plan9", "mips"
	if runtime.GOOS == "plan9" {
		os1 = "dragonfly"
	}
	return assembleAndPush(t, []oci.PlatformBinary{
		{OS: os1, Arch: arch1, Path: bin, Entrypoint: "github-repo"},
	})
}

func assembleAndPush(t *testing.T, bins []oci.PlatformBinary) *builtIndex {
	t.Helper()
	const version = "1.4.0"
	const coord = "ossf/pvtr-github-repo-scanner"
	idx, err := oci.AssembleIndex(oci.AssembleParams{
		Coordinate: coord,
		Plugin:     coord,
		Version:    version,
		Binaries:   bins,
		Evaluates:  []pluginspec.Evaluate{{Catalog: "ossf/osps.baseline", CatalogVersion: "2025.02", RequirementIDs: []string{"OSPS-AC-01"}}},
	})
	if err != nil {
		t.Fatalf("AssembleIndex: %v", err)
	}

	store := memory.New()
	if _, err := idx.PushTo(context.Background(), store); err != nil {
		t.Fatalf("PushTo memory store: %v", err)
	}
	return &builtIndex{
		store:    store,
		idxDesc:  idx.Index.Descriptor(),
		idxBytes: append([]byte(nil), idx.Index.Data...),
		version:  version,
		coord:    coord,
	}
}

// fetched builds an oci.FetchedIndex over the built index's store with the given
// (possibly nil) signature bundle bytes. A single bundle is wrapped in a slice;
// nil means unsigned (no bundles).
func (b *builtIndex) fetched(bundleJSON []byte) *oci.FetchedIndex {
	var bundles [][]byte
	if bundleJSON != nil {
		bundles = [][]byte{bundleJSON}
	}
	return oci.NewFetchedIndex(b.coord, b.version, b.idxDesc, b.idxBytes, bundles, b.store)
}

// fetchedMulti builds an oci.FetchedIndex with multiple signature bundles,
// for testing the multi-bundle selection logic.
func (b *builtIndex) fetchedMulti(bundleJSONs [][]byte) *oci.FetchedIndex {
	return oci.NewFetchedIndex(b.coord, b.version, b.idxDesc, b.idxBytes, bundleJSONs, b.store)
}

// signEntity signs the built index's bytes (whose sha256 IS the index digest)
// under the given SAN/issuer, returning a SignedEntity for verifyEntity. We sign
// the index BYTES so the verifier's WithArtifactDigest("sha256", <index digest>)
// matches — same construction the hub uses.
func (b *builtIndex) signEntity(t *testing.T, vs *ca.VirtualSigstore, san, issuer string) sgverify.SignedEntity {
	t.Helper()
	entity, err := vs.SignAtTime(san, issuer, b.idxBytes, time.Now())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return entity
}

func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// tamperingTarget wraps a read-only target and, for a chosen descriptor digest,
// serves DIFFERENT bytes than the descriptor commits to — simulating a malicious
// or corrupt registry. A content-addressed store (memory.Store) refuses to store
// mismatched bytes under a digest, so this is the only honest way to exercise the
// digest-walk's fail-closed checks. It preserves Predecessors so referrer
// discovery still works.
type tamperingTarget struct {
	inner     oras.ReadOnlyTarget
	tamperDig digest.Digest
	tamperBy  []byte
}

func (t *tamperingTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if desc.Digest == t.tamperDig {
		return io.NopCloser(bytes.NewReader(t.tamperBy)), nil
	}
	return t.inner.Fetch(ctx, desc)
}

func (t *tamperingTarget) Exists(ctx context.Context, desc ocispec.Descriptor) (bool, error) {
	return t.inner.Exists(ctx, desc)
}

func (t *tamperingTarget) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	return t.inner.Resolve(ctx, ref)
}

func (t *tamperingTarget) Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	if gs, ok := t.inner.(content.ReadOnlyGraphStorage); ok {
		return gs.Predecessors(ctx, node)
	}
	return nil, nil
}

// fetchedTampered builds a FetchedIndex whose target serves tampered bytes for
// the given descriptor digest.
func (b *builtIndex) fetchedTampered(tamperDig digest.Digest, tamperBy []byte) *oci.FetchedIndex {
	tgt := &tamperingTarget{inner: b.store, tamperDig: tamperDig, tamperBy: tamperBy}
	return oci.NewFetchedIndex(b.coord, b.version, b.idxDesc, b.idxBytes, nil, tgt)
}

func fetchFromStore(t *testing.T, b *builtIndex, desc ocispec.Descriptor) []byte {
	t.Helper()
	data, err := oci.FetchBytes(context.Background(), b.store, desc, 1<<30)
	if err != nil {
		t.Fatalf("fetch %s: %v", desc.Digest, err)
	}
	return data
}

// hostChildDesc returns the index child descriptor matching the host platform.
func hostChildDesc(t *testing.T, b *builtIndex) ocispec.Descriptor {
	t.Helper()
	var index ocispec.Index
	if err := json.Unmarshal(b.idxBytes, &index); err != nil {
		t.Fatal(err)
	}
	for _, c := range index.Manifests {
		if c.Platform != nil && c.Platform.OS == runtime.GOOS && c.Platform.Architecture == runtime.GOARCH {
			return c
		}
	}
	t.Fatal("no host child descriptor in built index")
	return ocispec.Descriptor{}
}

// hostChildAndLayer returns the host child's descriptor and its binary-layer
// descriptor.
func hostChildAndLayer(t *testing.T, b *builtIndex) (child, layer ocispec.Descriptor) {
	t.Helper()
	child = hostChildDesc(t, b)
	var m ocispec.Manifest
	if err := json.Unmarshal(fetchFromStore(t, b, child), &m); err != nil {
		t.Fatal(err)
	}
	for _, l := range m.Layers {
		if l.MediaType == oci.MediaTypePluginBinary {
			return child, l
		}
	}
	t.Fatal("no binary layer in host child")
	return ocispec.Descriptor{}, ocispec.Descriptor{}
}

// hostConfigAndLayer returns the host child's first layer and its config
// descriptor.
func hostConfigAndLayer(t *testing.T, b *builtIndex) (layer, config ocispec.Descriptor) {
	t.Helper()
	child := hostChildDesc(t, b)
	var m ocispec.Manifest
	if err := json.Unmarshal(fetchFromStore(t, b, child), &m); err != nil {
		t.Fatal(err)
	}
	return m.Layers[0], m.Config
}
