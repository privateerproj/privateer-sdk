package command

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// TestGetBinary_ResolvesViaManifest verifies that binary resolution reads the
// manifest by name+version: an unpinned package resolves to the latest installed
// version, an explicit pin resolves to that exact version, and a missing
// plugin/version errors.
func TestGetBinary_ResolvesViaManifest(t *testing.T) {
	dir := t.TempDir()
	viper.Set("binaries-path", dir)
	t.Cleanup(func() { viper.Set("binaries-path", "") })

	m := &manifest.Manifest{}
	m.Add(manifest.Plugin{Name: "ossf/scanner", Version: "1.0.0", BinaryPath: filepath.Join("ossf/scanner", "1.0.0", "scanner")})
	m.Add(manifest.Plugin{Name: "ossf/scanner", Version: "2.0.0", BinaryPath: filepath.Join("ossf/scanner", "2.0.0", "scanner")})
	if err := m.Save(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	// No pin → latest installed version.
	path, err := (&PluginPkg{Name: "ossf/scanner"}).getBinary()
	if err != nil {
		t.Fatalf("latest resolution: %v", err)
	}
	if want := filepath.Join(dir, "ossf/scanner", "2.0.0", "scanner"); path != want {
		t.Errorf("latest: got %q, want %q", path, want)
	}

	// Explicit pin → that exact version.
	path, err = (&PluginPkg{Name: "ossf/scanner", Version: "1.0.0"}).getBinary()
	if err != nil {
		t.Fatalf("pinned resolution: %v", err)
	}
	if want := filepath.Join(dir, "ossf/scanner", "1.0.0", "scanner"); path != want {
		t.Errorf("pinned: got %q, want %q", path, want)
	}

	// Pin to an uninstalled version → error.
	if _, err := (&PluginPkg{Name: "ossf/scanner", Version: "9.9.9"}).getBinary(); err == nil {
		t.Error("expected error for uninstalled pinned version")
	}

	// Unknown plugin → error.
	if _, err := (&PluginPkg{Name: "ossf/nonexistent"}).getBinary(); err == nil {
		t.Error("expected error for unknown plugin")
	}
}
