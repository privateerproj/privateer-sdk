package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/mod/semver"

	"github.com/privateerproj/privateer-sdk/utils"
)

const filename = "plugins.json"

// Plugin represents an installed plugin entry in the manifest.
//
// Coordinate, IndexDigest, and SignerIdentity apply only to grc.store-sourced
// installs (signed OCI indexes); they are omitempty so manifests written by the
// GitHub-Releases path don't carry empty grc.store keys.
type Plugin struct {
	Name       string `json:"name"`       // full owner/repo form, e.g. "ossf/pvtr-github-repo-scanner"
	Version    string `json:"version"`    // version installed from registry
	BinaryPath string `json:"binaryPath"` // filename relative to binaries-path

	// Coordinate is the grc.store plugin coordinate "<namespace>/<plugin_id>"
	// the binary was pulled from. Empty for GitHub-Releases-sourced plugins.
	Coordinate string `json:"coordinate,omitempty"`
	// IndexDigest is the verified OCI image-index digest (sha256:...) the
	// install was resolved from, recorded for update/re-verify drift detection.
	IndexDigest string `json:"indexDigest,omitempty"`
	// SignerIdentity is the normalized keyless signer identity
	// ("keyless:<issuer>#<workflow-path>") pinned on first install and enforced
	// on update (client-side TOFU). Empty for GitHub-Releases-sourced plugins.
	SignerIdentity string `json:"signerIdentity,omitempty"`
}

// Manifest tracks installed plugins, keyed by "<name>@<version>" so multiple
// installed versions of the same plugin can coexist.
type Manifest struct {
	Plugins map[string]Plugin `json:"plugins"`
}

// key is the manifest map key for a plugin entry. Keying by name+version (rather
// than name alone) is what lets two versions of the same plugin coexist.
func key(name, version string) string {
	return fmt.Sprintf("%s@%s", name, version)
}

// Load reads the manifest from {binariesPath}/plugins.json.
// Returns an empty manifest if the file does not exist.
//
// If the file contains the legacy array format (Plugins was []Plugin before it
// became map[string]Plugin), Load migrates it in-memory. The next Save writes
// the new map format, so the migration is transparent to callers.
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
		migrated, merr := migrateFromArray(data)
		if merr != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		return migrated, nil
	}
	return &m, nil
}

// migrateFromArray handles the legacy format where Plugins was a JSON array.
func migrateFromArray(data []byte) (*Manifest, error) {
	var legacy struct {
		Plugins []Plugin `json:"plugins"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	m := &Manifest{Plugins: make(map[string]Plugin, len(legacy.Plugins))}
	for _, p := range legacy.Plugins {
		m.Plugins[key(p.Name, p.Version)] = p
	}
	return m, nil
}

// Save writes the manifest to {binariesPath}/plugins.json atomically via
// utils.WriteFileAtomic (temp + rename) so a crash mid-write can never leave a
// partial manifest that causes the next run to error on JSON parse.
func (m *Manifest) Save(binariesPath string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}
	data = append(data, '\n')

	dest := filepath.Join(binariesPath, filename)
	return utils.WriteFileAtomic(dest, data, 0o644)
}

// updateMu serializes the read-modify-write performed by Update. The manifest is
// rewritten whole, so two unsynchronized Load -> mutate -> Save sequences clobber
// each other: the second Save overwrites the entry the first added. Update closes
// that window by re-reading inside the lock.
//
// This guards in-process concurrency only (multiple goroutines, e.g. the
// concurrent installer behind `pvtr install --from-config` / autoinstall). Two
// separate pvtr processes sharing one binaries dir would still need an OS-level
// file lock; that is intentionally out of scope here.
var updateMu sync.Mutex

// Update applies mutate to a freshly-loaded manifest and saves the result while
// holding updateMu, so concurrent in-process installers serialize cleanly. The
// load happens INSIDE the lock — callers must route mutations through Update
// rather than doing their own Load/mutate/Save, or they reintroduce the
// lost-update race Update exists to prevent.
func Update(binariesPath string, mutate func(*Manifest) error) error {
	updateMu.Lock()
	defer updateMu.Unlock()

	m, err := Load(binariesPath)
	if err != nil {
		return err
	}
	if err := mutate(m); err != nil {
		return err
	}
	return m.Save(binariesPath)
}

// Add upserts a plugin entry. The entry is keyed by name@version, so adding a
// new version of an already-installed plugin coexists with the old one, while
// re-adding an identical name+version replaces it in place.
func (m *Manifest) Add(p Plugin) {
	if m.Plugins == nil {
		m.Plugins = make(map[string]Plugin)
	}
	m.Plugins[key(p.Name, p.Version)] = p
}

// RemoveAllVersions deletes every entry for a plugin name, across all installed
// versions. No-op if the name is not present.
func (m *Manifest) RemoveAllVersions(name string) {
	for k, p := range m.Plugins {
		if p.Name == name {
			delete(m.Plugins, k)
		}
	}
}

// RemoveVersion deletes the entry for one specific name+version, leaving any
// other installed versions of the same plugin in place. No-op if that exact
// version is not present.
func (m *Manifest) RemoveVersion(name, version string) {
	delete(m.Plugins, key(name, version))
}

// Find returns the latest installed version of a plugin by its full owner/repo
// name, or nil if absent. It is a convenience wrapper over Latest for callers
// that want version-independent data (e.g. the pinned signer identity, which is
// the same across versions); Latest makes the choice deterministic.
func (m *Manifest) Find(name string) *Plugin {
	return m.Latest(name)
}

// FindVersion returns the entry for an exact name+version, or nil if absent.
func (m *Manifest) FindVersion(name, version string) *Plugin {
	if p, ok := m.Plugins[key(name, version)]; ok {
		return &p
	}
	return nil
}

// Latest returns the highest installed version of a plugin by name, or nil if
// none is installed. Versions are compared as semver (a leading "v" is supplied
// since manifests store bare versions like "1.4.0"); entries whose versions are
// not valid semver (e.g. local installs at version "local") fall back to lexical
// comparison.
func (m *Manifest) Latest(name string) *Plugin {
	var best *Plugin
	for k := range m.Plugins {
		p := m.Plugins[k]
		if p.Name != name {
			continue
		}
		if best == nil || compareVersions(p.Version, best.Version) > 0 {
			entry := p
			best = &entry
		}
	}
	return best
}

// FindByBinary returns every entry whose binary entrypoint (the basename of its
// BinaryPath) matches entrypoint — i.e. all installed version options that share
// that binary name. The result is empty when none match.
func (m *Manifest) FindByBinary(entrypoint string) []Plugin {
	var out []Plugin
	for k := range m.Plugins {
		p := m.Plugins[k]
		if filepath.Base(p.BinaryPath) == entrypoint {
			out = append(out, p)
		}
	}
	return out
}

// compareVersions orders two manifest version strings, preferring semver and
// falling back to lexical order for non-semver values.
func compareVersions(a, b string) int {
	av, bv := semverize(a), semverize(b)
	if semver.IsValid(av) && semver.IsValid(bv) {
		return semver.Compare(av, bv)
	}
	return strings.Compare(a, b)
}

// semverize prefixes a bare version with "v" so it can be passed to the semver
// package, which requires the leading "v".
func semverize(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
