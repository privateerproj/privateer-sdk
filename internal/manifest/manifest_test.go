package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

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
	content := `{"plugins":[{"name":"ossf/pvtr-scanner","version":"1.0.0","binaryPath":"pvtr-scanner"}]}`
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
	if m.Plugins[0].Name != "ossf/pvtr-scanner" {
		t.Errorf("expected name ossf/pvtr-scanner, got %s", m.Plugins[0].Name)
	}
	if m.Plugins[0].Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Plugins[0].Version)
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

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
			{Name: "privateerproj/pvtr-example", Version: "2.0.0", BinaryPath: "pvtr-example"},
		},
	}

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
	if loaded.Plugins[0].Name != "ossf/pvtr-scanner" {
		t.Errorf("plugin 0: expected ossf/pvtr-scanner, got %s", loaded.Plugins[0].Name)
	}
	if loaded.Plugins[1].Name != "privateerproj/pvtr-example" {
		t.Errorf("plugin 1: expected privateerproj/pvtr-example, got %s", loaded.Plugins[1].Name)
	}
}

func TestAdd_Insert(t *testing.T) {
	m := &Manifest{}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"})

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m.Plugins))
	}
	if m.Plugins[0].Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Plugins[0].Version)
	}
}

func TestAdd_Upsert(t *testing.T) {
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
		},
	}
	m.Add(Plugin{Name: "ossf/pvtr-scanner", Version: "2.0.0", BinaryPath: "pvtr-scanner"})

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after upsert, got %d", len(m.Plugins))
	}
	if m.Plugins[0].Version != "2.0.0" {
		t.Errorf("expected version 2.0.0 after upsert, got %s", m.Plugins[0].Version)
	}
}

func TestRemove(t *testing.T) {
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
			{Name: "privateerproj/pvtr-example", Version: "2.0.0", BinaryPath: "pvtr-example"},
		},
	}
	m.Remove("ossf/pvtr-scanner")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after remove, got %d", len(m.Plugins))
	}
	if m.Plugins[0].Name != "privateerproj/pvtr-example" {
		t.Errorf("wrong plugin remained: %s", m.Plugins[0].Name)
	}
}

func TestRemove_NotFound(t *testing.T) {
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
		},
	}
	m.Remove("nonexistent/plugin")

	if len(m.Plugins) != 1 {
		t.Fatalf("expected 1 plugin unchanged, got %d", len(m.Plugins))
	}
}

func TestFind(t *testing.T) {
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
		},
	}

	p := m.Find("ossf/pvtr-scanner")
	if p == nil {
		t.Fatal("expected to find plugin, got nil")
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", p.Version)
	}

	if m.Find("nonexistent") != nil {
		t.Error("expected nil for nonexistent plugin")
	}
}

func TestFindByBinary(t *testing.T) {
	m := &Manifest{
		Plugins: []Plugin{
			{Name: "ossf/pvtr-scanner", Version: "1.0.0", BinaryPath: "pvtr-scanner"},
		},
	}

	p := m.FindByBinary("pvtr-scanner")
	if p == nil {
		t.Fatal("expected to find plugin, got nil")
	}
	if p.Name != "ossf/pvtr-scanner" {
		t.Errorf("expected name ossf/pvtr-scanner, got %s", p.Name)
	}

	if m.FindByBinary("nonexistent") != nil {
		t.Error("expected nil for nonexistent binary")
	}
}
