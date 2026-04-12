package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const filename = "plugins.json"

// Plugin represents an installed plugin entry in the manifest.
type Plugin struct {
	Name       string `json:"name"`       // full owner/repo form, e.g. "ossf/pvtr-github-repo-scanner"
	Version    string `json:"version"`    // version installed from registry
	BinaryPath string `json:"binaryPath"` // filename relative to binaries-path
}

// Manifest tracks installed plugins.
type Manifest struct {
	Plugins []Plugin `json:"plugins"`
}

// Load reads the manifest from {binariesPath}/plugins.json.
// Returns an empty manifest if the file does not exist.
func Load(binariesPath string) (*Manifest, error) {
	p := filepath.Join(binariesPath, filename)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	return &m, nil
}

// Save writes the manifest to {binariesPath}/plugins.json atomically.
func (m *Manifest) Save(binariesPath string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}
	data = append(data, '\n')

	dest := filepath.Join(binariesPath, filename)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming %s to %s: %w", tmp, dest, err)
	}
	return nil
}

// Add upserts a plugin entry by name.
func (m *Manifest) Add(p Plugin) {
	for i, existing := range m.Plugins {
		if existing.Name == p.Name {
			m.Plugins[i] = p
			return
		}
	}
	m.Plugins = append(m.Plugins, p)
}

// Remove deletes a plugin entry by name.
func (m *Manifest) Remove(name string) {
	for i, p := range m.Plugins {
		if p.Name == name {
			m.Plugins = append(m.Plugins[:i], m.Plugins[i+1:]...)
			return
		}
	}
}

// Find looks up a plugin by its full owner/repo name.
func (m *Manifest) Find(name string) *Plugin {
	for i, p := range m.Plugins {
		if p.Name == name {
			return &m.Plugins[i]
		}
	}
	return nil
}

// FindByBinary looks up a plugin by its binary filename.
func (m *Manifest) FindByBinary(binaryName string) *Plugin {
	for i, p := range m.Plugins {
		if p.BinaryPath == binaryName {
			return &m.Plugins[i]
		}
	}
	return nil
}
