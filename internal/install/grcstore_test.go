package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/verify"
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

func TestPathContainmentCheck(t *testing.T) {
	destDir := "/safe/dir"
	cases := []struct {
		name    string
		relPath string
		escapes bool
	}{
		{"normal path", filepath.Join("ns/id", "1.0.0", "binary"), false},
		{"traversal attempt", filepath.Join("../escape", "1.0.0", "binary"), true},
		{"deep traversal", filepath.Join("ns/../../etc", "1.0.0", "binary"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest := filepath.Join(destDir, c.relPath)
			clean := filepath.Clean(dest) + string(filepath.Separator)
			base := filepath.Clean(destDir) + string(filepath.Separator)
			escaped := !strings.HasPrefix(clean, base)
			if escaped != c.escapes {
				t.Errorf("path %q: expected escapes=%v, got %v", c.relPath, c.escapes, escaped)
			}
		})
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

// The grc.store install records the digest of the bytes it writes, so run can
// detect a later change, and the entry counts as signed.
func TestVerifiedEntry_RecordsBinaryDigest(t *testing.T) {
	bin := []byte("verified-binary")
	got := verifiedEntry("ossf/scanner", "ossf/scanner/1.0.0/scanner", &verify.VerifiedPlugin{
		Version:        "1.0.0",
		IndexDigest:    "sha256:abc",
		SignerIdentity: "keyless:https://issuer#wf",
		Binary:         bin,
	})
	if got.BinaryDigest != manifest.DigestBytes(bin) {
		t.Errorf("BinaryDigest = %q, want %q", got.BinaryDigest, manifest.DigestBytes(bin))
	}
	if !got.Signed() {
		t.Error("entry from a verified install is not Signed()")
	}
	if got.Coordinate != "ossf/scanner" || got.Version != "1.0.0" {
		t.Errorf("got %+v", got)
	}
}

// --- commitVerifiedInstall tests -------------------------------------------

func testVerifiedPlugin() *verify.VerifiedPlugin {
	return &verify.VerifiedPlugin{
		Version:        "1.0.0",
		IndexDigest:    "sha256:abc",
		SignerIdentity: "keyless:https://issuer#wf",
		Binary:         []byte("verified-binary"),
	}
}

// A failed catalog install must leave neither the binary nor a manifest entry,
// so `install --from-config` and autoinstall still see the plugin as missing
// and retry it.
func TestCommitVerifiedInstall_CatalogFailureLeavesNothingInstalled(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join("ossf/scanner", "1.0.0", "scanner")
	var w bytes.Buffer

	err := commitVerifiedInstall(&w, dir, "ossf/scanner", rel, testVerifiedPlugin(), func() ([]error, error) {
		return nil, errors.New("registry unreachable")
	})
	if err == nil || !strings.Contains(err.Error(), "installing catalogs for ossf/scanner:1.0.0") {
		t.Fatalf("expected catalog install error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(statErr) {
		t.Errorf("binary was written despite the catalog failure (stat err: %v)", statErr)
	}
	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if m.Find("ossf/scanner") != nil {
		t.Error("manifest entry was written despite the catalog failure")
	}
	if strings.Contains(w.String(), "Successfully installed") {
		t.Errorf("reported success despite the catalog failure:\n%s", w.String())
	}
}

// Catalogs are installed before the binary exists; once they succeed, the
// binary and its signed entry are written and unpublished catalogs only warn.
func TestCommitVerifiedInstall_CatalogsFirstThenBinaryAndEntry(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join("ossf/scanner", "1.0.0", "scanner")
	var w bytes.Buffer

	err := commitVerifiedInstall(&w, dir, "ossf/scanner", rel, testVerifiedPlugin(), func() ([]error, error) {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Error("binary already written when catalogs were installed")
		}
		return []error{errors.New("catalog openssf/old@1 is not published")}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, rel)); err != nil || string(got) != "verified-binary" {
		t.Errorf("binary = %q, %v; want the verified bytes", got, err)
	}
	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if entry := m.Find("ossf/scanner"); entry == nil || !entry.Signed() {
		t.Errorf("expected a signed manifest entry, got %+v", entry)
	}
	out := w.String()
	if !strings.Contains(out, "Successfully installed ossf/scanner:1.0.0") || !strings.Contains(out, "Warning: catalog openssf/old@1 is not published") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

// An existing install must survive a failed catalog install on upgrade or
// reinstall: its binary, manifest entry, signer pin and digest stay as they
// were, and no entry is added for the new version.
func TestCommitVerifiedInstall_CatalogFailureKeepsExistingInstall(t *testing.T) {
	for _, tc := range []struct {
		name       string
		newVersion string
	}{
		{"upgrade to a new version", "2.0.0"},
		{"reinstall of the same version", "1.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			oldRel := filepath.Join("ossf/scanner", "1.0.0", "scanner")
			oldBinary := []byte("old-binary")
			if err := writeVerifiedBinary(dir, oldRel, oldBinary); err != nil {
				t.Fatal(err)
			}
			seeded := manifest.Plugin{
				Name:           "ossf/scanner",
				Version:        "1.0.0",
				BinaryPath:     oldRel,
				Coordinate:     "ossf/scanner",
				IndexDigest:    "sha256:old",
				SignerIdentity: "keyless:https://issuer#pinned",
				BinaryDigest:   manifest.DigestBytes(oldBinary),
			}
			if err := manifest.Update(dir, func(m *manifest.Manifest) error {
				m.Add(seeded)
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			newPlugin := testVerifiedPlugin()
			newPlugin.Version = tc.newVersion
			newRel := filepath.Join("ossf/scanner", tc.newVersion, "scanner")
			err := commitVerifiedInstall(&bytes.Buffer{}, dir, "ossf/scanner", newRel, newPlugin, func() ([]error, error) {
				return nil, errors.New("registry unreachable")
			})
			if err == nil {
				t.Fatal("expected catalog install error")
			}

			if got, err := os.ReadFile(filepath.Join(dir, oldRel)); err != nil || !bytes.Equal(got, oldBinary) {
				t.Errorf("existing binary = %q, %v; want it unchanged", got, err)
			}
			m, err := manifest.Load(dir)
			if err != nil {
				t.Fatalf("loading manifest: %v", err)
			}
			if got := m.FindVersion("ossf/scanner", "1.0.0"); got == nil || *got != seeded {
				t.Errorf("existing entry = %+v, want it unchanged: %+v", got, seeded)
			}
			if len(m.Plugins) != 1 {
				t.Errorf("manifest has %d entries, want only the existing one", len(m.Plugins))
			}
		})
	}
}
