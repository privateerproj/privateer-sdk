package command

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// `pvtr list --installed` builds its table from getLocalPlugins, so the
// "In Current Config" column is only right if getLocalPlugins marks the
// installed versions that a configured target would run.
func TestGetLocalPlugins_MarksConfiguredPlugins(t *testing.T) {
	dir := t.TempDir()
	viper.Set("binaries-path", dir)
	viper.Set("targets", map[string]any{
		"unpinned": map[string]any{"plugin": "ossf/scanner"},
		"pinned":   map[string]any{"plugin": "acme/checker", "version": "v1.0.0"},
	})
	t.Cleanup(func() {
		viper.Set("binaries-path", "")
		viper.Set("targets", nil)
	})

	m := &manifest.Manifest{}
	for _, p := range []manifest.Plugin{
		{Name: "ossf/scanner", Version: "1.0.0"},
		{Name: "ossf/scanner", Version: "2.0.0"},
		{Name: "acme/checker", Version: "1.0.0"},
		{Name: "acme/checker", Version: "2.0.0"},
		{Name: "local/unused", Version: "local"},
	} {
		p.BinaryPath = filepath.Join(p.Name, p.Version, "bin")
		m.Add(p)
	}
	if err := m.Save(dir); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	want := map[string]bool{
		"ossf/scanner@1.0.0": false, // an unpinned target runs the latest version only
		"ossf/scanner@2.0.0": true,
		"acme/checker@1.0.0": true, // the pinned version, matched despite the "v" prefix
		"acme/checker@2.0.0": false,
		"local/unused@local": false,
	}
	got := map[string]bool{}
	for _, p := range getLocalPlugins() {
		got[p.Name+"@"+p.Version] = p.Requested
	}
	for key, requested := range want {
		if got[key] != requested {
			t.Errorf("%s: Requested = %t, want %t", key, got[key], requested)
		}
	}
}
