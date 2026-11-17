package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// BinaryDigest is the sha256 digest ("sha256:<hex>") of the binary bytes
	// written at install time. `pvtr run` re-hashes the on-disk binary against
	// it just before exec (command.runOne via verifyBeforeExec), so a binary
	// modified after install is refused rather than run. Empty for entries
	// written before digests were recorded.
	BinaryDigest string `json:"binaryDigest,omitempty"`
}

// Signed reports whether the entry came from a signature-verified install: it
// carries both the verified index digest and the pinned signer identity. Local
// installs (`pvtr install <path>`) and legacy entries are not signed.
//
// Signed means "a valid keyless signature from some identity", not "from the
// expected publisher". On a first install where the hub declares no identity
// and there is no local pin, any valid Fulcio identity is accepted and pinned.
func (p Plugin) Signed() bool {
	return p.IndexDigest != "" && p.SignerIdentity != ""
}

// InstalledPlugins returns the plugins recorded in {binariesPath}/plugins.json.
// A missing manifest returns an empty list.
//
// Supports both the current map format (keyed by "name@version") and the
// legacy array format. The returned plugins are sorted deterministically by
// Name, then Version.
func InstalledPlugins(binariesPath string) ([]Plugin, error) {
	p := filepath.Join(binariesPath, filename)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Plugin{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", p, err)
	}

	var current struct {
		Plugins map[string]Plugin `json:"plugins"`
	}
	if err := json.Unmarshal(data, &current); err == nil && current.Plugins != nil {
		out := make([]Plugin, 0, len(current.Plugins))
		for _, plugin := range current.Plugins {
			out = append(out, plugin)
		}
		sortPlugins(out)
		return out, nil
	}

	var legacy struct {
		Plugins []Plugin `json:"plugins"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}

	seen := make(map[string]bool, len(legacy.Plugins))
	out := make([]Plugin, 0, len(legacy.Plugins))
	for _, plugin := range legacy.Plugins {
		key := fmt.Sprintf("%s@%s", plugin.Name, plugin.Version)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, plugin)
	}
	sortPlugins(out)
	return out, nil
}

func sortPlugins(plugins []Plugin) {
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].Name != plugins[j].Name {
			return plugins[i].Name < plugins[j].Name
		}
		return plugins[i].Version < plugins[j].Version
	})
}
