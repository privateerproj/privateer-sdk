package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestSave_OmitsEmptyGRCStoreFields guards the omitempty contract: saving a
// plugin that carries no grc.store provenance must not inject coordinate/
// indexDigest/signerIdentity keys.
func TestSave_OmitsEmptyGRCStoreFields(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-github-repo-scanner", Version: "1.4.0", BinaryPath: "github-repo"})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("reading saved manifest: %v", err)
	}
	for _, k := range [][]byte{[]byte("coordinate"), []byte("indexDigest"), []byte("signerIdentity")} {
		if bytes.Contains(data, k) {
			t.Errorf("saved manifest unexpectedly contains %q key:\n%s", k, data)
		}
	}
}

func TestLoad_MissingFile(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Plugins) != 0 {
		t.Errorf("expected empty plugins, got %d", len(m.Plugins))
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	content := `{"plugins":{"ossf/pvtr-scanner@1.0.0":{"name":"ossf/pvtr-scanner","version":"1.0.0","binaryPath":"pvtr-scanner"}}}`
	if err := os.WriteFile(filepath.Join(dir, "plugins.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m.Plugins))
	}
	p := m.Find("ossf/pvtr-scanner")
	if p == nil {
		t.Fatal("expected to find ossf/pvtr-scanner, got nil")
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", p.Version)
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugins.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestLoad_LegacyArrayFormat(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"plugins":[{"name":"ossf/pvtr-scanner","version":"1.0.0","binaryPath":"pvtr-scanner"},{"name":"acme/foo","version":"2.0.0","binaryPath":"foo"}]}`
	if err := os.WriteFile(filepath.Join(dir, "plugins.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("expected legacy array to migrate successfully, got: %v", err)
	}
	if len(m.Plugins) != 2 {
		t.Fatalf("expected 2 migrated plugins, got %d", len(m.Plugins))
	}
	if p := m.Find("ossf/pvtr-scanner"); p == nil || p.Version != "1.0.0" {
		t.Errorf("expected migrated ossf/pvtr-scanner@1.0.0, got %+v", p)
	}
	if p := m.Find("acme/foo"); p == nil || p.Version != "2.0.0" {
		t.Errorf("expected migrated acme/foo@2.0.0, got %+v", p)
	}
}

func TestLoad_LegacyArrayFormat_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"plugins":[{"name":"ossf/pvtr-scanner","version":"1.0.0","binaryPath":"pvtr-scanner"}]}`
	if err := os.WriteFile(filepath.Join(dir, "plugins.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save after migration: %v", err)
	}
	m2, err := Load(dir)
	if err != nil {
		t.Fatalf("Re-load after save: %v", err)
	}
	if len(m2.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after round-trip, got %d", len(m2.Plugins))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.Add(Plugin{Name: "privateerproj/pvtr-example", Version: "2.0.0", BinaryPath: "pvtr-example"})

	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(loaded.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(loaded.Plugins))
	}
	if p := loaded.Find("ossf/pvtr-scanner"); p == nil || p.Version != "1.0.0" {
		t.Errorf("ossf/pvtr-scanner not preserved: %+v", p)
	}
	if p := loaded.Find("privateerproj/pvtr-example"); p == nil || p.Version != "2.0.0" {
		t.Errorf("privateerproj/pvtr-example not preserved: %+v", p)
	}
}

func TestAdd_Insert(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m.Plugins))
	}
	if p := m.Find("ossf/pvtr-scanner"); p == nil || p.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %+v", p)
	}
}

// TestAdd_Upsert: re-adding the same name+version replaces in place.
func TestAdd_Upsert(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "renamed"})

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after same-version re-add, got %d", len(m.Plugins))
	}
	if p := m.Find("ossf/pvtr-scanner"); p == nil || p.BinaryPath != "renamed" {
		t.Errorf("expected BinaryPath updated to 'renamed', got %+v", p)
	}
}

// TestAdd_MultipleVersions: adding a new version of an installed plugin keeps
// both entries — the reason Plugins is keyed by name@version.
func TestAdd_MultipleVersions(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "pvtr-scanner"})

	if len(m.Plugins) != 2 {
		t.Fatalf("expected 2 coexisting versions, got %d", len(m.Plugins))
	}
	if _, ok := m.Plugins["ossf/pvtr-scanner@1.0.0"]; !ok {
		t.Error("expected v1.0.0 entry to remain")
	}
	if _, ok := m.Plugins["ossf/pvtr-scanner@2.0.0"]; !ok {
		t.Error("expected v2.0.0 entry to be added")
	}
}

func TestRemoveAllVersions(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.Add(Plugin{Name: "privateerproj/pvtr-example", Version: "2.0.0", BinaryPath: "pvtr-example"})
	m.RemoveAllVersions("ossf/pvtr-scanner")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after remove, got %d", len(m.Plugins))
	}
	if m.Find("privateerproj/pvtr-example") == nil {
		t.Error("wrong plugin removed: privateerproj/pvtr-example should remain")
	}
}

// TestRemoveAllVersions_DropsEveryVersion: removing by name drops all versions.
func TestRemoveAllVersions_DropsEveryVersion(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "pvtr-scanner"})
	m.RemoveAllVersions("ossf/pvtr-scanner")

	if len(m.Plugins) != 0 {
		t.Fatalf("expected all versions removed, got %d", len(m.Plugins))
	}
}

func TestRemoveAllVersions_NotFound(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.RemoveAllVersions("nonexistent/plugin")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin unchanged, got %d", len(m.Plugins))
	}
}

// TestRemoveVersion removes one version and leaves the plugin's other versions.
func TestRemoveVersion(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "ossf/pvtr-scanner/1.0.0/pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "ossf/pvtr-scanner/2.0.0/pvtr-scanner"})
	m.RemoveVersion("ossf/pvtr-scanner", "1.0.0")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 remaining version, got %d", len(m.Plugins))
	}
	if m.FindVersion("ossf/pvtr-scanner", "1.0.0") != nil {
		t.Error("v1.0.0 should have been removed")
	}
	if m.FindVersion("ossf/pvtr-scanner", "2.0.0") == nil {
		t.Error("v2.0.0 should remain")
	}
}

// TestRemoveVersion_NotFound: removing an absent version is a no-op.
func TestRemoveVersion_NotFound(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})
	m.RemoveVersion("ossf/pvtr-scanner", "9.9.9")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin unchanged, got %d", len(m.Plugins))
	}
}

// TestFind: Find returns the latest installed version (it delegates to Latest).
func TestFind(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "ossf/pvtr-scanner/1.0.0/pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "ossf/pvtr-scanner/2.0.0/pvtr-scanner"})

	p := m.Find("ossf/pvtr-scanner")
	if p == nil {
		t.Fatal("expected to find plugin, got nil")
	}
	if p.Version != "2.0.0" {
		t.Errorf("expected latest version 2.0.0, got %s", p.Version)
	}

	if m.Find("nonexistent") != nil {
		t.Error("expected nil for nonexistent plugin")
	}
}

// TestLatest covers semver ordering and the non-semver lexical fallback.
func TestLatest(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.9.0", BinaryPath: "a"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.10.0", BinaryPath: "b"}) // semver: 1.10 > 1.9
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.2.0", BinaryPath: "c"})

	if got := m.Latest("ossf/pvtr-scanner"); got == nil || got.Version != "1.10.0" {
		t.Errorf("expected latest 1.10.0 (semver, not lexical), got %+v", got)
	}
	if m.Latest("nonexistent") != nil {
		t.Error("expected nil latest for nonexistent plugin")
	}

	// Non-semver versions (e.g. local) fall back to lexical comparison.
	local := &Manifest{}
	local.Add(Plugin{Name: "local/x", Version: "local", BinaryPath: "local/x"})
	if got := local.Latest("local/x"); got == nil || got.Version != "local" {
		t.Errorf("expected the sole local entry, got %+v", got)
	}
}

func TestFindVersion(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "ossf/pvtr-scanner/1.0.0/pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "ossf/pvtr-scanner/2.0.0/pvtr-scanner"})

	if p := m.FindVersion("ossf/pvtr-scanner", "1.0.0"); p == nil || p.BinaryPath != "ossf/pvtr-scanner/1.0.0/pvtr-scanner" {
		t.Errorf("expected v1.0.0 entry, got %+v", p)
	}
	if m.FindVersion("ossf/pvtr-scanner", "9.9.9") != nil {
		t.Error("expected nil for an uninstalled version")
	}
}

// TestFindByBinary returns every installed version sharing a binary entrypoint
// (the basename of BinaryPath), now that versions live in per-version subdirs.
func TestFindByBinary(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "ossf/pvtr-scanner/1.0.0/pvtr-scanner"})
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "ossf/pvtr-scanner/2.0.0/pvtr-scanner"})

	got := m.FindByBinary("pvtr-scanner")
	if len(got) != 2 {
		t.Fatalf("expected 2 version options sharing the entrypoint, got %d", len(got))
	}
	for _, p := range got {
		if p.Name != "ossf/pvtr-scanner" {
			t.Errorf("expected name ossf/pvtr-scanner, got %s", p.Name)
		}
	}

	if len(m.FindByBinary("nonexistent")) != 0 {
		t.Error("expected empty result for nonexistent binary")
	}
}

// TestUpdate_ConcurrentAddsDoNotClobber proves Update serializes the whole
// read-modify-write: N goroutines each adding a distinct entry must all survive.
// A plain Load -> Add -> Save would lose entries to last-writer-wins; run with
// -race to also catch unsynchronized access.
func TestUpdate_ConcurrentAddsDoNotClobber(t *testing.T) {
	dir := t.TempDir()
	const n = 25

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Update(dir, func(m *Manifest) error {
				m.Add(Plugin{
					Name:       fmt.Sprintf("acme/p%d", i),
					Version:    "1.0.0",
					BinaryPath: fmt.Sprintf("acme/p%d/1.0.0/p%d", i, i),
				})
				return nil
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Update %d failed: %v", i, err)
		}
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after concurrent updates: %v", err)
	}
	if len(m.Plugins) != n {
		t.Fatalf("expected %d entries after %d concurrent Updates, got %d", n, n, len(m.Plugins))
	}
}

// TestUpdate_MutateErrorIsNotSaved confirms a mutate that returns an error aborts
// the write — the manifest on disk is left untouched.
func TestUpdate_MutateErrorIsNotSaved(t *testing.T) {
	dir := t.TempDir()
	if err := Update(dir, func(m *Manifest) error {
		m.Add(Plugin{Name: "acme/should-not-persist", Version: "1.0.0", BinaryPath: "x"})
		return fmt.Errorf("boom")
	}); err == nil {
		t.Fatal("expected Update to surface the mutate error")
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Plugins) != 0 {
		t.Fatalf("a failed mutate must not write; got %d entries", len(m.Plugins))
	}
}
