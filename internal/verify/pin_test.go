package verify

import (
	"strings"
	"testing"
)

func TestPinnedIdentityFor_NoLocalPinNoHub(t *testing.T) {
	pin, warn := PinnedIdentityFor("", "", "uninstall first")
	if pin != "" {
		t.Errorf("want empty pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

func TestPinnedIdentityFor_NoLocalPinHubProvided(t *testing.T) {
	pin, warn := PinnedIdentityFor("", "keyless:https://issuer#workflow", "uninstall first")
	if pin != "keyless:https://issuer#workflow" {
		t.Errorf("want hub identity as pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

func TestPinnedIdentityFor_LocalPinMatchesHub(t *testing.T) {
	id := "keyless:https://issuer#workflow"
	pin, warn := PinnedIdentityFor(id, id, "uninstall first")
	if pin != id {
		t.Errorf("want matching pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning when pin matches hub, got %q", warn)
	}
}

func TestPinnedIdentityFor_LocalPinDiffersFromHub(t *testing.T) {
	localID := "keyless:https://issuer#old-workflow"
	hubID := "keyless:https://issuer#new-workflow"
	pin, warn := PinnedIdentityFor(localID, hubID, "delete the cached catalog to accept the new identity")
	if pin != localID {
		t.Errorf("local pin must win, got %q", pin)
	}
	if warn == "" {
		t.Fatal("expected a warning when local pin differs from hub identity")
	}
	if !strings.Contains(warn, localID) || !strings.Contains(warn, hubID) {
		t.Errorf("warning should mention both identities, got: %q", warn)
	}
	if !strings.Contains(warn, "delete the cached catalog to accept the new identity") {
		t.Errorf("warning should include action hint, got: %q", warn)
	}
}

