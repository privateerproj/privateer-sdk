package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// installFixture writes a binary and a manifest entry for it, returning the
// binaries dir and the binary's on-disk path.
func installFixture(t *testing.T, entry manifest.Plugin, content []byte) (string, string) {
	t.Helper()
	dir := t.TempDir()
	viper.Set("binaries-path", dir)
	t.Cleanup(func() {
		viper.Set("binaries-path", "")
		viper.Set("require-signed-plugins", false)
	})
	bin := filepath.Join(dir, entry.BinaryPath)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, content, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{}
	m.Add(entry)
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	return dir, bin
}

func signedEntry(content []byte) manifest.Plugin {
	return manifest.Plugin{
		Name:           "ossf/scanner",
		Version:        "1.0.0",
		BinaryPath:     filepath.Join("ossf/scanner", "1.0.0", "scanner"),
		Coordinate:     "ossf/scanner",
		IndexDigest:    "sha256:" + strings.Repeat("a", 64),
		SignerIdentity: "keyless:https://token.actions.githubusercontent.com#wf",
		BinaryDigest:   manifest.DigestBytes(content),
	}
}

func TestGetBinary_DigestMatchRuns(t *testing.T) {
	content := []byte("plugin-bytes")
	_, bin := installFixture(t, signedEntry(content), content)
	viper.Set("require-signed-plugins", true)

	got, err := (&PluginPkg{Name: "ossf/scanner"}).getBinary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != bin {
		t.Errorf("got %q, want %q", got, bin)
	}
}

// The hash runs at exec time, not plan time: getBinary resolves a tampered
// binary, and runOne refuses it before starting the plugin.
func TestRunOne_TamperedBinaryRefused(t *testing.T) {
	content := []byte("plugin-bytes")
	_, bin := installFixture(t, signedEntry(content), content)

	pkg := NewPluginPkg("ossf/scanner", "", "s3")
	if pkg.Error != nil || !pkg.Installed {
		t.Fatalf("plan time: got error %v, installed %t; want resolved", pkg.Error, pkg.Installed)
	}
	if err := os.WriteFile(bin, []byte("evil-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, fatal := runOne(hclog.NewNullLogger(), 1, pkg)
	if code != BadUsage || fatal {
		t.Fatalf("got (%d, %t), want (%d, false)", code, fatal, BadUsage)
	}
	if !errors.Is(pkg.Error, ErrPluginVerification) {
		t.Fatalf("got %v, want ErrPluginVerification", pkg.Error)
	}
	msg := pkg.Error.Error()
	for _, want := range []string{"tampered", "pvtr install ossf/scanner@1.0.0", "autoinstall will not replace"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestGetBinary_RequireSignedRefusesLocal(t *testing.T) {
	content := []byte("local-bytes")
	entry := manifest.Plugin{
		Name:         "local/scanner",
		Version:      "local",
		BinaryPath:   filepath.Join("local", "scanner"),
		BinaryDigest: manifest.DigestBytes(content),
	}
	installFixture(t, entry, content)

	// Allowed by default.
	if _, err := (&PluginPkg{Name: "local/scanner"}).getBinary(); err != nil {
		t.Fatalf("default policy: unexpected error %v", err)
	}

	viper.Set("require-signed-plugins", true)
	_, err := (&PluginPkg{Name: "local/scanner"}).getBinary()
	if err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("got %v, want not-signed error", err)
	}
}

func TestGetBinary_LegacyEntryWithoutDigest(t *testing.T) {
	content := []byte("plugin-bytes")
	entry := signedEntry(content)
	entry.BinaryDigest = ""
	installFixture(t, entry, content)

	// Backward compatible by default: nothing recorded to check.
	if _, err := (&PluginPkg{Name: "ossf/scanner"}).getBinary(); err != nil {
		t.Fatalf("default policy: unexpected error %v", err)
	}

	viper.Set("require-signed-plugins", true)
	_, err := (&PluginPkg{Name: "ossf/scanner"}).getBinary()
	if err == nil || !strings.Contains(err.Error(), "no recorded binary digest") {
		t.Fatalf("got %v, want missing-digest error", err)
	}
}

// A plugin refused by require-signed-plugins must be reported as refused (with
// the reason), not as "not installed": the latter would send the user to
// install something that is already installed.
func TestPlanRun_ReportsRefusedNotMissing(t *testing.T) {
	content := []byte("local-bytes")
	installFixture(t, manifest.Plugin{
		Name:         "local/scanner",
		Version:      "local",
		BinaryPath:   filepath.Join("local", "scanner"),
		BinaryDigest: manifest.DigestBytes(content),
	}, content)
	viper.Set("require-signed-plugins", true)

	pkg := NewPluginPkg("local/scanner", "", "s3")
	pkg.Requested = true
	missing := &PluginPkg{Name: "acme/absent", ServiceTarget: "other", Requested: true}

	_, code, msg := planRun([]*PluginPkg{pkg, missing}, "")
	if code != BadUsage {
		t.Fatalf("exit code: got %d, want %d", code, BadUsage)
	}
	if !strings.Contains(msg, "refusing to run") || !strings.Contains(msg, "not signed") {
		t.Errorf("message does not report the refusal: %q", msg)
	}
	if strings.Contains(msg, "local/scanner (required by") {
		t.Errorf("refused plugin misreported as not installed: %q", msg)
	}
	if !strings.Contains(msg, "acme/absent") {
		t.Errorf("genuinely missing plugin not reported: %q", msg)
	}
}

// `pvtr list` shows a tampered plugin as refused, not as installed or missing.
func TestList_ShowsTamperedAsRefused(t *testing.T) {
	content := []byte("plugin-bytes")
	_, bin := installFixture(t, signedEntry(content), content)
	if err := os.WriteFile(bin, []byte("evil-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}

	plugins := getLocalPlugins()
	markRefused(plugins)
	var out bytes.Buffer
	renderPluginDetails(&out, plugins)
	if !strings.Contains(out.String(), "refused") {
		t.Errorf("list output does not show the plugin as refused:\n%s", out.String())
	}
}

// An unchanged plugin still lists as installed.
func TestList_CleanPluginInstalled(t *testing.T) {
	content := []byte("plugin-bytes")
	installFixture(t, signedEntry(content), content)

	plugins := getLocalPlugins()
	markRefused(plugins)
	if len(plugins) != 1 || !plugins[0].Installed || plugins[0].Error != nil {
		t.Fatalf("got %+v, want one installed plugin with no error", plugins)
	}
}

// An SDK consumer can build its own PluginPkg and skip getBinary. With
// require-signed-plugins on, runOne must refuse it rather than exec it.
func TestRunOne_HandBuiltPkgRefusedUnderPolicy(t *testing.T) {
	viper.Set("require-signed-plugins", true)
	t.Cleanup(func() { viper.Set("require-signed-plugins", false) })

	pkg := &PluginPkg{Name: "rogue", Path: "/bin/sh", Installed: true, Requested: true}
	code, fatal := runOne(hclog.NewNullLogger(), 1, pkg)
	if code != BadUsage || fatal {
		t.Fatalf("got (%d, %t), want (%d, false)", code, fatal, BadUsage)
	}
	if !errors.Is(pkg.Error, ErrPluginVerification) {
		t.Fatalf("got %v, want ErrPluginVerification", pkg.Error)
	}
}

// Without the policy, a hand-built PluginPkg keeps working as before.
func TestVerifyBeforeExec_NoEntryNoPolicy(t *testing.T) {
	viper.Set("require-signed-plugins", false)
	if err := (&PluginPkg{Name: "dev", Path: "/bin/sh"}).verifyBeforeExec(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A deleted binary is missing, not tampered with: it gets a reinstall hint and
// is not reported as refused.
func TestVerifyBinaryDigest_MissingBinaryIsNotRefused(t *testing.T) {
	content := []byte("plugin-bytes")
	_, bin := installFixture(t, signedEntry(content), content)
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}

	plugins := getLocalPlugins()
	markRefused(plugins)
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}
	err := plugins[0].Error
	if err == nil || errors.Is(err, ErrPluginVerification) {
		t.Fatalf("got %v, want a non-verification error", err)
	}
	if !strings.Contains(err.Error(), "pvtr install ossf/scanner@1.0.0") {
		t.Errorf("error %q has no reinstall hint", err)
	}
	if got := installedStatus(plugins[0]); got != "false" {
		t.Errorf("list status = %q, want false", got)
	}
}

// Two pinned versions of one plugin, both refused, keep both reasons.
func TestRefusedPluginsMsg_KeepsEachVersion(t *testing.T) {
	v1 := &PluginPkg{Name: "ossf/scanner", Version: "1.0.0", Error: errors.New("reason-one")}
	v2 := &PluginPkg{Name: "ossf/scanner", Version: "2.0.0", Error: errors.New("reason-two")}
	dup := &PluginPkg{Name: "ossf/scanner", Version: "1.0.0", Error: errors.New("reason-one")}

	msg := refusedPluginsMsg([]*PluginPkg{v1, v2, dup})
	if !strings.Contains(msg, "reason-one") || !strings.Contains(msg, "reason-two") {
		t.Errorf("message dropped a version: %q", msg)
	}
	if strings.Count(msg, "reason-one") != 1 {
		t.Errorf("same version reported twice: %q", msg)
	}
}
