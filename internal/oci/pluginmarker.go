package oci

import (
	"bytes"
	"fmt"
	"os"

	"github.com/privateerproj/privateer-sdk/shared"
)

// IsPrivateerPlugin reports whether the binary at path embeds the Privateer
// go-plugin handshake marker — i.e. it serves via the SDK's shared.Serve and is
// therefore a binary `pvtr run` can actually execute. The check is a STATIC byte
// scan (the binary is NEVER executed — we can't run foreign-arch binaries, and
// we won't exec untrusted bytes at publish time).
//
// The marker is the go-plugin magic cookie (key + value) from
// shared.GetHandshakeConfig() — compiled string literals that survive `-s -w`
// stripping (verified against a real, stripped pvtr-github-repo-scanner build).
// The cookie strings are pulled from `shared` (DRY) — never hardcoded here, so
// they can't drift from what the host (`pvtr run`) handshakes with.
func IsPrivateerPlugin(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading binary %s: %w", path, err)
	}
	return bytesHavePluginMarker(data), nil
}

// bytesHavePluginMarker is the pure scan: the binary must contain BOTH the magic
// cookie key and value. Requiring both (not just the generic key) keeps a random
// binary that happens to contain one token from passing.
func bytesHavePluginMarker(data []byte) bool {
	hc := shared.GetHandshakeConfig()
	return bytes.Contains(data, []byte(hc.MagicCookieKey)) &&
		bytes.Contains(data, []byte(hc.MagicCookieValue))
}

// ValidatePluginBinaries rejects a publish whose dist contains a binary that is
// not a Privateer plugin (missing the handshake marker) — catching a dist that
// points at the wrong build, or a binary that doesn't serve via shared.Serve,
// BEFORE anything is pushed or signed. It scans every resolved binary; a single
// non-plugin fails the whole publish.
func ValidatePluginBinaries(bins []PlatformBinary) error {
	// Distinct paths only (the darwin universal is referenced by two entries).
	seen := map[string]bool{}
	for _, b := range bins {
		if seen[b.Path] {
			continue
		}
		seen[b.Path] = true
		ok, err := IsPrivateerPlugin(b.Path)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s is not a Privateer plugin (missing the go-plugin handshake marker) — did --dist point at the right build? a plugin must serve via the SDK (shared.Serve)", b.Path)
		}
	}
	return nil
}
