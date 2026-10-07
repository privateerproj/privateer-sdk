package command

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/shared"
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

func TestCloseClient_NonPassExitWithoutError(t *testing.T) {
	tests := []struct {
		name       string
		pkg        PluginPkg
		want       string
		wantAbsent string
	}{
		{"pass", PluginPkg{Successful: true}, "completed successfully", "Unexpected"},
		{"error", PluginPkg{Error: errors.New("boom")}, "Error from svc", "Unexpected"},
		{"test fail without error", PluginPkg{ExitCode: shared.TestFail}, "finished with TestFail", "Unexpected exit"},
		{"unknown code", PluginPkg{ExitCode: 99}, "Unexpected exit from svc", "finished with"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := hclog.New(&hclog.LoggerOptions{Output: &buf, Level: hclog.Info})
			client := hcplugin.NewClient(&hcplugin.ClientConfig{
				HandshakeConfig: shared.GetHandshakeConfig(),
				Cmd:             exec.Command("true"),
			})
			tt.pkg.closeClient("svc", client, logger)
			out := buf.String()
			if !strings.Contains(out, tt.want) {
				t.Errorf("expected %q in %q", tt.want, out)
			}
			if strings.Contains(out, tt.wantAbsent) {
				t.Errorf("did not expect %q in %q", tt.wantAbsent, out)
			}
		})
	}
}
