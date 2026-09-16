package verify

import (
	"context"
	"encoding/json"
	"errors"
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
	store := memory.New()
	desc, err := bundle.Pack(context.Background(), store, &bundle.Bundle{
		Source: bundle.File{Name: "baseline.gemara.yaml", Type: "ControlCatalog", Data: []byte(testCatalogYAML)},
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
	if _, err := artifactLayer([]ocispec.Descriptor{art, art}); !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("ambiguous layers: %v", err)
	}
	if _, err := artifactLayer([]ocispec.Descriptor{other}); !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("no artifact layer: %v", err)
	}
}
