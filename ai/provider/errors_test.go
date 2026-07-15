package provider

import (
	"context"
	"errors"
	"testing"
)

// TestClassifyTransportError_ContextCanceled locks in the behavior that a
// cancelled request context maps to ErrorKindTimeout, so callers branching on
// Kind never retry a request the user explicitly cancelled. See PR #218 review
// discussion: https://github.com/privateerproj/privateer-sdk/pull/218#discussion_r3327923013
func TestClassifyTransportError_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := classifyTransportError(testProvider, ctx.Err())
	if err == nil {
		t.Fatal("expected non-nil error from classifyTransportError")
	}

	var aiErr *Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if aiErr.Kind != ErrorKindTimeout {
		t.Errorf("Kind = %q, want %q", aiErr.Kind, ErrorKindTimeout)
	}
	if aiErr.Provider != testProvider {
		t.Errorf("Provider = %q, want %q", aiErr.Provider, testProvider)
	}
	if !errors.Is(aiErr, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; underlying error not preserved: %v", aiErr.Err)
	}
}

// TestClassifyTransportError_ContextDeadlineExceeded is the deadline-side
// counterpart to TestClassifyTransportError_ContextCanceled and pins the
// existing DeadlineExceeded -> Timeout mapping so neither branch regresses.
func TestClassifyTransportError_ContextDeadlineExceeded(t *testing.T) {
	err := classifyTransportError(testProvider, context.DeadlineExceeded)

	var aiErr *Error
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if aiErr.Kind != ErrorKindTimeout {
		t.Errorf("Kind = %q, want %q", aiErr.Kind, ErrorKindTimeout)
	}
	if !errors.Is(aiErr, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; underlying error not preserved: %v", aiErr.Err)
	}
}
