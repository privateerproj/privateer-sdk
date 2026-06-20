package verify

import (
	"testing"

	"github.com/revanite-io/grc-store-protocol/identity"
)

// TestCanonicalIdentity_MatchesLivePinnedValue is a guard against drift in the
// grc-store-protocol identity package: it pins the exact canonical string the
// preview hub TOFU-recorded (and pvtr verified byte-for-byte) for the live CI
// publish of sandbox/sandbox-plugin@0.26.1-rc. The SAN carries the per-run
// @refs/tags ref, which must be stripped to the workflow path. If the module
// ever changes this output, our verify path's TOFU compare would silently stop
// matching what the hub stored — this test fails closed first.
func TestCanonicalIdentity_MatchesLivePinnedValue(t *testing.T) {
	const (
		issuer = "https://token.actions.githubusercontent.com"
		san    = "https://github.com/eddie-knight/sandbox-plugin/.github/workflows/grcstore-publish.yml@refs/tags/v0.26.1-rc"
		want   = "keyless:https://token.actions.githubusercontent.com#https://github.com/eddie-knight/sandbox-plugin/.github/workflows/grcstore-publish.yml"
	)
	if got := identity.CanonicalKeylessIdentity(issuer, san); got != want {
		t.Fatalf("canonical identity drift:\n got  %q\n want %q", got, want)
	}
}
