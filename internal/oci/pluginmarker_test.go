package oci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/privateerproj/privateer-sdk/shared"
)

func TestBytesHavePluginMarker(t *testing.T) {
	hc := shared.GetHandshakeConfig()

	// A binary embeds BOTH the cookie key and value (as shared.Serve does).
	plugin := []byte("...random prefix..." + hc.MagicCookieKey + "...gap..." + hc.MagicCookieValue + "...suffix...")
	if !bytesHavePluginMarker(plugin) {
		t.Error("a binary with both cookie key and value must be recognized as a plugin")
	}

	// A non-plugin binary has neither.
	if bytesHavePluginMarker([]byte("just some random ELF/Mach-O bytes, no marker")) {
		t.Error("a binary without the marker must NOT be recognized as a plugin")
	}

	// Only one token present (e.g. an unrelated binary that happens to contain
	// the key string) must NOT pass — we require both.
	if bytesHavePluginMarker([]byte("contains " + hc.MagicCookieKey + " only")) {
		t.Error("requiring both tokens: key-only must not pass")
	}
	if bytesHavePluginMarker([]byte("contains " + hc.MagicCookieValue + " only")) {
		t.Error("requiring both tokens: value-only must not pass")
	}
}

func TestIsPrivateerPlugin_File(t *testing.T) {
	hc := shared.GetHandshakeConfig()
	dir := t.TempDir()

	pluginPath := filepath.Join(dir, "plugin-bin")
	if err := os.WriteFile(pluginPath, []byte("hdr"+hc.MagicCookieKey+"x"+hc.MagicCookieValue), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsPrivateerPlugin(pluginPath); err != nil || !ok {
		t.Errorf("plugin binary: ok=%v err=%v", ok, err)
	}

	nonPath := filepath.Join(dir, "non-plugin")
	if err := os.WriteFile(nonPath, []byte("a plain go hello-world, no handshake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsPrivateerPlugin(nonPath); err != nil || ok {
		t.Errorf("non-plugin binary should be rejected: ok=%v err=%v", ok, err)
	}

	if _, err := IsPrivateerPlugin(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file must error")
	}
}

func TestValidatePluginBinaries(t *testing.T) {
	hc := shared.GetHandshakeConfig()
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(good, []byte(hc.MagicCookieKey+hc.MagicCookieValue), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	// All plugins → ok. The darwin-universal dedup (two entries, one path) is
	// also exercised here (good appears twice).
	if err := ValidatePluginBinaries([]PlatformBinary{
		{OS: "linux", Arch: "amd64", Path: good},
		{OS: "darwin", Arch: "amd64", Path: good},
		{OS: "darwin", Arch: "arm64", Path: good},
	}); err != nil {
		t.Errorf("all-plugin set should pass: %v", err)
	}

	// One non-plugin → the whole set is rejected, naming the offender.
	err := ValidatePluginBinaries([]PlatformBinary{
		{OS: "linux", Arch: "amd64", Path: good},
		{OS: "windows", Arch: "amd64", Path: bad},
	})
	if err == nil {
		t.Fatal("a set containing a non-plugin must be rejected")
	}
	if !contains(err.Error(), bad) || !contains(err.Error(), "not a Privateer plugin") {
		t.Errorf("error should name the offending binary + cause, got: %v", err)
	}
}
