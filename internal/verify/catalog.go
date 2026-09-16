package verify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gemaraproj/go-gemara/bundle"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/privateerproj/privateer-sdk/internal/oci"
)

// artifactRoleAnnotation marks the layer that carries the catalog itself in a
// Gemara bundle manifest (the other layers are resolved imports). go-gemara
// keeps the key unexported, so it is redeclared here, matching the hub.
const artifactRoleAnnotation = "org.gemara.artifact.role"

// VerifiedCatalog is the trusted result of verifying a catalog pulled from
// grc.store: the YAML bytes to cache and the signer to report. The bytes
// come from the digest-checked layer of the signed manifest.
type VerifiedCatalog struct {
	SignerIdentity string // canonical keyless identity of the signer
	YAML           []byte
}

// Catalog verifies a fetched Gemara catalog manifest: the signature and signer
// identity first (nothing in the manifest is trusted before that), then the
// manifest bytes against their digest, then the artifact layer against the
// digest the manifest commits to. A catalog is a single image manifest with
// one YAML layer, so there is no platform walk. Any failure aborts.
func (v *Verifier) Catalog(ctx context.Context, fetched *oci.FetchedIndex, policy IdentityPolicy) (*VerifiedCatalog, error) {
	if fetched == nil {
		return nil, fmt.Errorf("%w: nil fetched manifest", ErrMalformedIndex)
	}
	signerIdentity, err := v.signer(ctx, fetched, policy)
	if err != nil {
		return nil, err
	}
	return walkVerifiedCatalog(ctx, fetched, signerIdentity)
}

// walkVerifiedCatalog runs the digest walk after the signature has been
// verified: manifest bytes, then the artifact layer. Split from Catalog so
// tests can drive the walk without a signature bundle.
func walkVerifiedCatalog(ctx context.Context, fetched *oci.FetchedIndex, signerIdentity string) (*VerifiedCatalog, error) {
	if err := checkDigest(fetched.IndexDescriptor.Digest, fetched.IndexBytes, "catalog manifest"); err != nil {
		return nil, err
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(fetched.IndexBytes, &m); err != nil {
		return nil, fmt.Errorf("%w: parse catalog manifest: %v", ErrMalformedIndex, err)
	}
	if m.MediaType != ocispec.MediaTypeImageManifest {
		return nil, fmt.Errorf("%w: catalog media type %q is not an image manifest", ErrMalformedIndex, m.MediaType)
	}
	layer, err := artifactLayer(m.Layers)
	if err != nil {
		return nil, err
	}
	data, err := oci.FetchBytes(ctx, fetched.Target(), layer, maxBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog layer: %w", err)
	}
	if err := checkDigest(layer.Digest, data, "catalog layer"); err != nil {
		return nil, err
	}
	return &VerifiedCatalog{SignerIdentity: signerIdentity, YAML: data}, nil
}

// artifactLayer picks the layer carrying the catalog: the first layer annotated
// with the artifact role, else — when none is annotated — the only Gemara
// artifact layer. Ambiguity is an error among unannotated layers only; an
// annotation is taken as the manifest saying which layer it means.
func artifactLayer(layers []ocispec.Descriptor) (ocispec.Descriptor, error) {
	var candidates []ocispec.Descriptor
	for _, l := range layers {
		if l.Annotations[artifactRoleAnnotation] == "artifact" {
			return l, nil
		}
		if l.MediaType == bundle.MediaTypeArtifact {
			candidates = append(candidates, l)
		}
	}
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return ocispec.Descriptor{}, fmt.Errorf("%w: no %q layer", ErrMalformedIndex, bundle.MediaTypeArtifact)
	default:
		return ocispec.Descriptor{}, fmt.Errorf("%w: %d artifact layers and none marked as the catalog", ErrMalformedIndex, len(candidates))
	}
}
