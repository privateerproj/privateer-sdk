package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestManifest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}
}

func TestInstalledPlugins_MissingFile(t *testing.T) {
	dir := t.TempDir()
	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestInstalledPlugins_NonExistentDirectory(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	plugins, err := InstalledPlugins(nonExistent)
	if err != nil {
		t.Fatalf("unexpected error for non-existent directory: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestInstalledPlugins_ValidMapFormat(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"plugins": {
			"ossf/scanner@1.0.0": {
				"name": "ossf/scanner",
				"version": "1.0.0",
				"binaryPath": "ossf/scanner/1.0.0/scanner",
				"coordinate": "ossf/scanner",
				"indexDigest": "sha256:abcd",
				"signerIdentity": "keyless:https://token#ref"
			},
			"acme/plugin@2.0.0": {
				"name": "acme/plugin",
				"version": "2.0.0",
				"binaryPath": "acme/plugin/2.0.0/plugin"
			}
		}
	}`
	writeTestManifest(t, dir, content)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	// Should be sorted deterministically: "acme/plugin" before "ossf/scanner"
	if plugins[0].Name != "acme/plugin" || plugins[0].Version != "2.0.0" {
		t.Errorf("unexpected first plugin: %+v", plugins[0])
	}
	if plugins[1].Name != "ossf/scanner" || plugins[1].Version != "1.0.0" {
		t.Errorf("unexpected second plugin: %+v", plugins[1])
	}
	if plugins[1].Coordinate != "ossf/scanner" || plugins[1].IndexDigest != "sha256:abcd" || plugins[1].SignerIdentity != "keyless:https://token#ref" {
		t.Errorf("expected grc.store fields preserved, got: %+v", plugins[1])
	}
}

func TestInstalledPlugins_LegacyArrayFormat(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"plugins": [
			{"name": "ossf/scanner", "version": "1.0.0", "binaryPath": "scanner"},
			{"name": "acme/plugin", "version": "0.5.0", "binaryPath": "plugin"}
		]
	}`
	writeTestManifest(t, dir, content)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	// Sorted: acme/plugin then ossf/scanner
	if plugins[0].Name != "acme/plugin" || plugins[0].Version != "0.5.0" {
		t.Errorf("unexpected first plugin: %+v", plugins[0])
	}
	if plugins[1].Name != "ossf/scanner" || plugins[1].Version != "1.0.0" {
		t.Errorf("unexpected second plugin: %+v", plugins[1])
	}
}

func TestInstalledPlugins_LegacyArrayDeduplication(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"plugins": [
			{"name": "ossf/scanner", "version": "1.0.0", "binaryPath": "scanner"},
			{"name": "ossf/scanner", "version": "1.0.0", "binaryPath": "scanner"},
			{"name": "ossf/scanner", "version": "2.0.0", "binaryPath": "scanner2"}
		]
	}`
	writeTestManifest(t, dir, content)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 deduplicated plugins, got %d", len(plugins))
	}
	if plugins[0].Version != "1.0.0" || plugins[1].Version != "2.0.0" {
		t.Errorf("unexpected versions after dedup: %+v", plugins)
	}
}

func TestInstalledPlugins_EmptyManifest(t *testing.T) {
	dir := t.TempDir()
	writeTestManifest(t, dir, `{"plugins": {}}`)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for empty map, got %d", len(plugins))
	}
}

func TestInstalledPlugins_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	writeTestManifest(t, dir, `{}`)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for empty object, got %d", len(plugins))
	}
}

func TestInstalledPlugins_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	writeTestManifest(t, dir, `not-json-content`)

	_, err := InstalledPlugins(dir)
	if err == nil {
		t.Fatal("expected error for corrupted JSON, got nil")
	}
}

func TestInstalledPlugins_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// Create a directory where plugins.json would be so ReadFile returns an error
	if err := os.Mkdir(filepath.Join(dir, filename), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err := InstalledPlugins(dir)
	if err == nil {
		t.Fatal("expected error for unreadable manifest path, got nil")
	}
}

func TestInstalledPlugins_SortingDeterministic(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"plugins": {
			"z/tool@1.0.0": {"name": "z/tool", "version": "1.0.0"},
			"a/tool@2.0.0": {"name": "a/tool", "version": "2.0.0"},
			"a/tool@1.0.0": {"name": "a/tool", "version": "1.0.0"},
			"m/tool@0.1.0": {"name": "m/tool", "version": "0.1.0"}
		}
	}`
	writeTestManifest(t, dir, content)

	plugins, err := InstalledPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 4 {
		t.Fatalf("expected 4 plugins, got %d", len(plugins))
	}

	expected := []struct {
		name    string
		version string
	}{
		{"a/tool", "1.0.0"},
		{"a/tool", "2.0.0"},
		{"m/tool", "0.1.0"},
		{"z/tool", "1.0.0"},
	}

	for i, exp := range expected {
		if plugins[i].Name != exp.name || plugins[i].Version != exp.version {
			t.Errorf("at index %d: got %s@%s, want %s@%s", i, plugins[i].Name, plugins[i].Version, exp.name, exp.version)
		}
	}
}
