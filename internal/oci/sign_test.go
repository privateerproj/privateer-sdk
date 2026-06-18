package oci

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content/memory"
)

// assembleTiny builds a minimal one-platform index and pushes it to a memory
// store, returning the index + store for referrer tests.
func assembleTiny(t *testing.T) (*AssembledIndex, *memory.Store) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "b")
	if err := os.WriteFile(bin, []byte("plugin-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	idx, err := AssembleIndex(AssembleParams{
		Coordinate: "acme/hello",
		Plugin:     "acme/hello",
		Version:    "0.1.0",
		Binaries:   []PlatformBinary{{OS: "linux", Arch: "amd64", Path: bin, Entrypoint: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	if _, err := idx.PushTo(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	return idx, store
}

// AttachSignature must be the EXACT inverse of fetchSignatureBundle: attaching a
// bundle as the index's OCI 1.1 referrer, then discovering it, yields the same
// bytes. This is the producer↔consumer mechanic the e2e relies on (real-Fulcio
// crypto is exercised there; the signature bytes here are an opaque fixture).
func TestAttachSignature_RoundTripsThroughDiscovery(t *testing.T) {
	ctx := context.Background()
	idx, store := assembleTiny(t)

	bundleJSON := []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","fixture":true}`)
	if err := AttachSignature(ctx, store, idx.Index.descriptor(), &SignedBundle{JSON: bundleJSON}); err != nil {
		t.Fatalf("AttachSignature: %v", err)
	}

	// fetchSignatureBundle (the consumer's discovery) must find exactly these bytes
	// as the sole element of the returned slice.
	got, _, err := fetchSignatureBundle(ctx, store, idx.Index.descriptor())
	if err != nil {
		t.Fatalf("fetchSignatureBundle: %v", err)
	}
	if len(got) != 1 || string(got[0]) != string(bundleJSON) {
		t.Errorf("round-trip mismatch:\n got %v\nwant [%s]", got, bundleJSON)
	}
}

// Before any signature is attached, discovery returns nil (unsigned), and after
// attach it returns the bundle — proving the referrer graph is what flips the
// index from "unsigned" to "signed" for the verify path.
func TestAttachSignature_UnsignedBeforeAttach(t *testing.T) {
	ctx := context.Background()
	idx, store := assembleTiny(t)

	if got, _, err := fetchSignatureBundle(ctx, store, idx.Index.descriptor()); err != nil || len(got) != 0 {
		t.Fatalf("expected no bundle before attach, got %v err %v", got, err)
	}
	if err := AttachSignature(ctx, store, idx.Index.descriptor(), &SignedBundle{JSON: []byte(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
	got, _, err := fetchSignatureBundle(ctx, store, idx.Index.descriptor())
	if err != nil || len(got) == 0 {
		t.Fatalf("expected a bundle after attach, got %v err %v", got, err)
	}
}

// SignIndex fails closed without a signing identity (the public-good Fulcio
// token), and that token is NOT the registry bearer.
func TestSignIndex_RequiresIDToken(t *testing.T) {
	idx, _ := assembleTiny(t)
	if _, err := SignIndex(context.Background(), idx, SignerOptions{}); err == nil {
		t.Fatal("expected error with no signing ID token")
	}
}
