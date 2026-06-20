package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// --- pinnedIdentityFor tests -----------------------------------------------

func TestPinnedIdentityFor_NoLocalPinNoHub(t *testing.T) {
	// No local pin, no hub identity → open TOFU (empty pin, no warning).
	pin, warn := pinnedIdentityFor(nil, "")
	if pin != "" {
		t.Errorf("want empty pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

func TestPinnedIdentityFor_NoLocalPinHubProvided(t *testing.T) {
	// No local pin but hub has a declared identity → seed from hub (first install).
	pin, warn := pinnedIdentityFor(nil, "keyless:https://issuer#workflow")
	if pin != "keyless:https://issuer#workflow" {
		t.Errorf("want hub identity as pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

func TestPinnedIdentityFor_LocalPinNopHubEmpty(t *testing.T) {
	// Local pin exists, hub has no declared identity → enforce local pin, no warning.
	existing := &manifest.Plugin{SignerIdentity: "keyless:https://issuer#workflow"}
	pin, warn := pinnedIdentityFor(existing, "")
	if pin != "keyless:https://issuer#workflow" {
		t.Errorf("want local pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

func TestPinnedIdentityFor_LocalPinMatchesHub(t *testing.T) {
	// Local pin matches hub identity → enforce local pin, no warning.
	id := "keyless:https://issuer#workflow"
	existing := &manifest.Plugin{SignerIdentity: id}
	pin, warn := pinnedIdentityFor(existing, id)
	if pin != id {
		t.Errorf("want matching pin, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning when pin matches hub, got %q", warn)
	}
}

func TestPinnedIdentityFor_LocalPinDiffersFromHub(t *testing.T) {
	// Local pin differs from hub identity → enforce local pin and emit a warning.
	localID := "keyless:https://issuer#old-workflow"
	hubID := "keyless:https://issuer#new-workflow"
	existing := &manifest.Plugin{SignerIdentity: localID}
	pin, warn := pinnedIdentityFor(existing, hubID)
	if pin != localID {
		t.Errorf("local pin must win, got %q", pin)
	}
	if warn == "" {
		t.Fatal("expected a warning when local pin differs from hub identity")
	}
	if !strings.Contains(warn, localID) || !strings.Contains(warn, hubID) {
		t.Errorf("warning should mention both identities, got: %q", warn)
	}
}

func TestPinnedIdentityFor_EmptySignerIdentityInExistingEntry(t *testing.T) {
	// A manifest entry with empty SignerIdentity (GitHub-Releases era) is
	// treated as "no local pin" — fall back to hub.
	existing := &manifest.Plugin{Name: "old/plugin", SignerIdentity: ""}
	pin, warn := pinnedIdentityFor(existing, "keyless:https://issuer#workflow")
	if pin != "keyless:https://issuer#workflow" {
		t.Errorf("want hub identity, got %q", pin)
	}
	if warn != "" {
		t.Errorf("want no warning, got %q", warn)
	}
}

// --- writeVerifiedBinary layout tests ---------------------------------------

// Two versions of the same plugin write to distinct per-version paths and so
// coexist instead of overwriting one another. This per-(name,version) layout is
// what replaced the old binary-collision guard: distinct plugins and versions
// can never resolve to the same file.
func TestWriteVerifiedBinary_VersionsCoexist(t *testing.T) {
	dir := t.TempDir()
	v1 := filepath.Join("acme/myplugin", "1.0.0", "myplugin")
	v2 := filepath.Join("acme/myplugin", "2.0.0", "myplugin")

	if err := writeVerifiedBinary(dir, v1, []byte("one")); err != nil {
		t.Fatalf("writing v1: %v", err)
	}
	if err := writeVerifiedBinary(dir, v2, []byte("two")); err != nil {
		t.Fatalf("writing v2: %v", err)
	}

	for path, want := range map[string]string{v1: "one", v2: "two"} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q (versions must not overwrite)", path, got, want)
		}
	}
}

// A nested relative path (coordinate/version/entrypoint) has its full parent
// chain created on write.
func TestWriteVerifiedBinary_CreatesNestedDirs(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join("ns/id", "3.2.1", "entry")
	if err := writeVerifiedBinary(dir, rel, []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
		t.Errorf("expected binary at nested path: %v", err)
	}
}
