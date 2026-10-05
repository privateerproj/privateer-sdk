package command

import (
	"path/filepath"
	"slices"
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

func TestQueueCmd_PassesBinariesPathToPlugin(t *testing.T) {
	viper.Set("binaries-path", "/opt/pvtr/bin")
	t.Cleanup(func() { viper.Set("binaries-path", "") })
	p := &PluginPkg{Path: "/bin/true", ServiceTarget: "svc"}
	p.queueCmd()
	found := false
	for _, kv := range p.Command.Env {
		if kv == "PVTR_BINARIES_PATH=/opt/pvtr/bin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plugin env lacks PVTR_BINARIES_PATH: %v", p.Command.Env)
	}
}

func TestQueueCmd_ForwardsRunFlagsToPlugin(t *testing.T) {
	viper.Set("write-directory", "/tmp/results")
	viper.Set("output", "json")
	viper.Set("write", false)
	viper.Set("include-payload", true)
	t.Cleanup(func() {
		for _, k := range []string{"write-directory", "output", "write", "include-payload"} {
			viper.Set(k, nil)
		}
	})
	p := &PluginPkg{Path: "/bin/true", ServiceTarget: "svc"}
	p.queueCmd()
	want := []string{
		"PVTR_WRITE_DIRECTORY=/tmp/results",
		"PVTR_OUTPUT=json",
		"PVTR_WRITE=false",
		"PVTR_INCLUDE_PAYLOAD=true",
	}
	for _, w := range want {
		if !slices.Contains(p.Command.Env, w) {
			t.Errorf("plugin env lacks %s: %v", w, p.Command.Env)
		}
	}
}
