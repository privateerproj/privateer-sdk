package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/memory"
)

// fetchSignatureBundle returns (nil,nil) for a target with no referrers — the
// "unsigned" condition is a verify-time concern, not a fetch error.
func TestFetchSignatureBundle_NoReferrerIsUnsigned(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	// Push a trivial manifest to act as the "index" and tag it.
	man := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.DescriptorEmptyJSON,
	}
	data, _ := json.Marshal(man)
	b := newBlob(ocispec.MediaTypeImageManifest, data)
	if err := store.Push(ctx, b.descriptor(), bytes.NewReader(b.Data)); err != nil {
		t.Fatal(err)
	}

	bundles, truncated, err := fetchSignatureBundle(ctx, store, b.descriptor())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundles) != 0 {
		t.Errorf("expected no bundles for no referrer, got %d", len(bundles))
	}
	if truncated {
		t.Error("expected truncated=false when there are no referrers")
	}
}

// ErrNotIndex is a typed, checkable error.
func TestErrNotIndex_IsTyped(t *testing.T) {
	wrapped := errors.Join(ErrNotIndex, errors.New("context"))
	if !errors.Is(wrapped, ErrNotIndex) {
		t.Error("ErrNotIndex must be checkable via errors.Is")
	}
}

// pushBlobIntoStore is a helper that pushes a blob into a memory store and
// returns its descriptor, for use in FetchBytes tests.
func pushBlobIntoStore(t *testing.T, store *memory.Store, data []byte) ocispec.Descriptor {
	t.Helper()
	b := newBlob("application/octet-stream", data)
	desc := b.descriptor()
	if err := store.Push(context.Background(), desc, bytes.NewReader(data)); err != nil {
		t.Fatalf("pushBlobIntoStore: %v", err)
	}
	return desc
}

// FetchBytes succeeds when the content is exactly at the limit.
func TestFetchBytes_AtLimit_Succeeds(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	const limit int64 = 10
	data := bytes.Repeat([]byte("x"), int(limit)) // exactly 10 bytes
	desc := pushBlobIntoStore(t, store, data)

	got, err := FetchBytes(ctx, store, desc, limit)
	if err != nil {
		t.Fatalf("expected no error for content at limit, got: %v", err)
	}
	if int64(len(got)) != limit {
		t.Errorf("expected %d bytes, got %d", limit, len(got))
	}
}

// FetchBytes returns the size error when content is one byte over the limit.
// The error must be explicit (not a digest mismatch) so the caller can
// distinguish a size-limit condition from tampering.
func TestFetchBytes_OneBytePastLimit_Errors(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	const limit int64 = 10
	data := bytes.Repeat([]byte("x"), int(limit)+1) // 11 bytes — one over
	desc := pushBlobIntoStore(t, store, data)

	_, err := FetchBytes(ctx, store, desc, limit)
	if err == nil {
		t.Fatal("expected error for over-limit content, got nil")
	}
	if !strings.Contains(err.Error(), "content exceeds maximum allowed size") {
		t.Errorf("expected size-limit error, got: %v", err)
	}
}

// lyingFetcher returns more bytes than the descriptor declares, simulating a
// registry whose response body exceeds the descriptor's stated Size (or omits
// it). It exercises the stream-side limit+1 guard, which the desc.Size
// pre-check cannot catch.
type lyingFetcher struct{ body []byte }

func (f lyingFetcher) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

// FetchBytes errors via the stream guard (not the desc.Size pre-check) when the
// actual content overruns the limit but the descriptor understates its size.
func TestFetchBytes_StreamOverrunWithUnderstatedSize_Errors(t *testing.T) {
	const limit int64 = 10
	// Descriptor claims size 0 (passes the desc.Size pre-check) but the body is
	// 15 bytes — the limit+1 read must detect the overrun.
	desc := ocispec.Descriptor{MediaType: "application/octet-stream", Size: 0}
	f := lyingFetcher{body: bytes.Repeat([]byte("z"), int(limit)+5)}

	_, err := FetchBytes(context.Background(), f, desc, limit)
	if err == nil {
		t.Fatal("expected error for stream overrun with understated size, got nil")
	}
	if !strings.Contains(err.Error(), "content exceeds maximum allowed size") {
		t.Errorf("expected size-limit error, got: %v", err)
	}
}

// FetchBytes short-circuits when the descriptor's declared Size already exceeds
// the limit, skipping the fetch entirely. This prevents a needless round-trip
// for obviously-oversized content.
func TestFetchBytes_DescriptorDeclaredOversize_ErrorsWithoutFetch(t *testing.T) {
	// Use a store that holds no content — if Fetch is called it will error for a
	// different reason (not found), which would mask the pre-check failure.
	store := memory.New()
	ctx := context.Background()

	const limit int64 = 10
	// Build a descriptor whose declared size exceeds the limit without pushing
	// any matching content into the store.
	oversizeDesc := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Size:      limit + 1,
		// Digest must be non-empty for a valid descriptor shape, but the
		// pre-check fires before any network/store access.
		Digest: newBlob("application/octet-stream", bytes.Repeat([]byte("y"), int(limit)+1)).Digest,
	}

	_, err := FetchBytes(ctx, store, oversizeDesc, limit)
	if err == nil {
		t.Fatal("expected error for descriptor-declared oversize, got nil")
	}
	if !strings.Contains(err.Error(), "content exceeds maximum allowed size") {
		t.Errorf("expected size-limit error, got: %v", err)
	}
}
