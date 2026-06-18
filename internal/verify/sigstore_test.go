package verify

import (
	"context"
	"errors"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
)

// The embedded public-good trusted root must parse and yield a usable verifier
// (guards against a corrupt/empty trusted_root.json embed — that would make
// every install fail closed in production).
func TestNewVerifier_EmbeddedRootParses(t *testing.T) {
	v, err := NewVerifier()
	if err != nil {
		t.Fatalf("NewVerifier with embedded public-good root: %v", err)
	}
	if v == nil {
		t.Fatal("nil verifier")
	}
}

// A bad index-digest string is a signature-invalid condition (fails closed),
// not a panic.
func TestVerifyEntity_BadDigestFormat(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	b := buildHostIndex(t)
	v := testVerifier(t, vs)
	entity := b.signEntity(t, vs, testSANRef, testIssuer)

	_, err = v.verifyEntity(context.Background(), entity, "not-a-sha256-digest")
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid for bad digest, got %v", err)
	}
}

// verifySignature with nil bundle bytes is ErrUnsigned (distinct from invalid).
func TestVerifySignature_NilBundleIsUnsigned(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	v := testVerifier(t, vs)
	_, err = v.verifySignature(context.Background(), nil, "sha256:"+
		"0000000000000000000000000000000000000000000000000000000000000000")
	if !errors.Is(err, ErrUnsigned) {
		t.Fatalf("expected ErrUnsigned, got %v", err)
	}
}
