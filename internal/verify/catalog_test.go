package verify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara/bundle"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"oras.land/oras-go/v2/content/memory"
)

const testCatalogYAML = "metadata:\n  id: osps-baseline\n  version: v1\ncontrols: []\n"

// packCatalog packs a one-layer Gemara catalog bundle into a memory store the
// way grcli publishes one, returning the store, the manifest descriptor, and
// the manifest bytes.
func packCatalog(t *testing.T) (*memory.Store, ocispec.Descriptor, []byte) {
	t.Helper()
	return packCatalogYAML(t, testCatalogYAML)
}

func packCatalogYAML(t *testing.T, yaml string) (*memory.Store, ocispec.Descriptor, []byte) {
	t.Helper()
	store := memory.New()
	desc, err := bundle.Pack(context.Background(), store, &bundle.Bundle{
		Source: bundle.File{Name: "baseline.gemara.yaml", Type: "ControlCatalog", Data: []byte(yaml)},
	})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	data, err := oci.FetchBytes(context.Background(), store, desc, 1<<20)
	if err != nil {
		t.Fatalf("fetch manifest: %v", err)
	}
	return store, desc, data
}

func TestCatalog_WalkHappyPath(t *testing.T) {
	store, desc, data := packCatalog(t)
	fetched := oci.NewFetchedIndex("openssf/osps-baseline", "v1", desc, data, nil, store)
	vc, err := walkVerifiedCatalog(context.Background(), fetched, "keyless:x#y")
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if string(vc.YAML) != testCatalogYAML {
		t.Errorf("yaml = %q", vc.YAML)
	}
	if vc.SignerIdentity != "keyless:x#y" {
		t.Errorf("signer = %q", vc.SignerIdentity)
	}
}

func TestCatalog_TamperedLayerRejected(t *testing.T) {
	store, desc, data := packCatalog(t)
	var m ocispec.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	tgt := &tamperingTarget{inner: store, tamperDig: m.Layers[0].Digest, tamperBy: []byte("controls: [evil]\n")}
	fetched := oci.NewFetchedIndex("openssf/osps-baseline", "v1", desc, data, nil, tgt)
	_, err := walkVerifiedCatalog(context.Background(), fetched, "id")
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch, got %v", err)
	}
}

func TestCatalog_UnsignedRejected(t *testing.T) {
	store, desc, data := packCatalog(t)
	v, err := NewVerifier()
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.Catalog(context.Background(), oci.NewFetchedIndex("openssf/osps-baseline", "v1", desc, data, nil, store), IdentityPolicy{})
	if !errors.Is(err, ErrUnsigned) {
		t.Fatalf("expected ErrUnsigned, got %v", err)
	}
}

func TestArtifactLayer(t *testing.T) {
	art := ocispec.Descriptor{MediaType: bundle.MediaTypeArtifact, Digest: "sha256:a"}
	marked := ocispec.Descriptor{MediaType: bundle.MediaTypeArtifact, Digest: "sha256:b", Annotations: map[string]string{artifactRoleAnnotation: "artifact"}}
	other := ocispec.Descriptor{MediaType: "application/octet-stream", Digest: "sha256:c"}

	if got, err := artifactLayer([]ocispec.Descriptor{art, marked}); err != nil || got.Digest != "sha256:b" {
		t.Errorf("marked layer should win: %v %v", got.Digest, err)
	}
	if got, err := artifactLayer([]ocispec.Descriptor{other, art}); err != nil || got.Digest != "sha256:a" {
		t.Errorf("single artifact layer should be chosen: %v %v", got.Digest, err)
	}
	if _, err := artifactLayer([]ocispec.Descriptor{art, art}); !errors.Is(err, ErrMalformedCatalog) {
		t.Errorf("ambiguous layers: %v", err)
	}
	if _, err := artifactLayer([]ocispec.Descriptor{other}); !errors.Is(err, ErrMalformedCatalog) {
		t.Errorf("no artifact layer: %v", err)
	}
	// The annotation does not override the media type, and only one layer may
	// claim the role — the same rules as go-gemara's bundle.Unpack.
	badMarked := ocispec.Descriptor{MediaType: "application/octet-stream", Digest: "sha256:d", Annotations: map[string]string{artifactRoleAnnotation: "artifact"}}
	if got, err := artifactLayer([]ocispec.Descriptor{badMarked, art}); err != nil || got.Digest != "sha256:a" {
		t.Errorf("annotated layer of the wrong media type must be ignored: %v %v", got.Digest, err)
	}
	if _, err := artifactLayer([]ocispec.Descriptor{marked, marked}); !errors.Is(err, ErrMalformedCatalog) {
		t.Errorf("two marked layers: %v", err)
	}
}

// A validly signed catalog for some other id must not verify under this
// coordinate, whether or not the hub recorded a manifest digest.
func TestCatalog_WrongCoordinateRejected(t *testing.T) {
	store, desc, data := packCatalog(t)
	fetched := oci.NewFetchedIndex("openssf/other-catalog", "v1", desc, data, nil, store)
	_, err := walkVerifiedCatalog(context.Background(), fetched, "id")
	if !errors.Is(err, ErrMalformedCatalog) || !strings.Contains(err.Error(), "osps-baseline") {
		t.Fatalf("expected a coordinate mismatch, got %v", err)
	}
}

// The hub coordinate id is the slug of metadata.id, so a mixed-case id such as
// the FINOS CCC catalogs carry must verify under its slugged coordinate.
func TestCatalog_SluggedCoordinateAccepted(t *testing.T) {
	store, desc, data := packCatalogYAML(t, "metadata:\n  id: CCC.ObjStor.CN\n  version: v1\ncontrols: []\n")
	fetched := oci.NewFetchedIndex("finos-ccc/ccc.objstor.cn", "v1", desc, data, nil, store)
	if _, err := walkVerifiedCatalog(context.Background(), fetched, "id"); err != nil {
		t.Fatalf("walk: %v", err)
	}
}
