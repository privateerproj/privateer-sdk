package harness

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// flushWriter is a Writer (io.Writer + Flush) that discards everything, for Run
// tests that need a flushable writer but don't inspect the output.
type flushWriter struct{}

func (flushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (flushWriter) Flush() error                { return nil }

// configureRun sets the viper state ensureRequestedInstalled reads: the
// autoinstall flag, the binaries dir, and one service requesting plugin@version
// (version empty => unpinned). It resets viper afterwards so tests don't leak.
func configureRun(t *testing.T, autoInstall bool, binDir, plugin, version string) {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set("autoinstall", autoInstall)
	viper.Set("binaries-path", binDir)
	if plugin != "" {
		svc := map[string]interface{}{"plugin": plugin}
		if version != "" {
			svc["version"] = version
		}
		viper.Set("services", map[string]interface{}{"svc": svc})
	}
}

// When autoinstall is off the preflight is a no-op even with a requested plugin
// that is not installed — the missing plugin is left to surface at Run.
func TestEnsureRequestedInstalled_NoopWhenDisabled(t *testing.T) {
	configureRun(t, false, t.TempDir(), "acme/missing", "")
	if err := ensureRequestedInstalled(context.Background(), nil); err != nil {
		t.Fatalf("disabled preflight should be a no-op, got: %v", err)
	}
}

// Autoinstall on, but the requested plugin is already in the manifest, so no
// install is attempted (the hub is never contacted — a contact would fail here).
func TestEnsureRequestedInstalled_SkipsInstalled(t *testing.T) {
	binDir := t.TempDir()
	m := &manifest.Manifest{}
	m.Add(manifest.Plugin{Name: "acme/hello", Version: "1.0.0", BinaryPath: "acme/hello/1.0.0/hello"})
	if err := m.Save(binDir); err != nil {
		t.Fatalf("seeding manifest: %v", err)
	}

	configureRun(t, true, binDir, "acme/hello", "")
	// Point the hub at a server that fails the test if reached: a skipped install
	// must not make any hub call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("hub must not be contacted when the plugin is already installed")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	viper.Set("hub-url", srv.URL)

	if err := ensureRequestedInstalled(context.Background(), io.Discard); err != nil {
		t.Fatalf("installed plugin should be skipped, got: %v", err)
	}
}

// Autoinstall on and the plugin is missing => the preflight resolves it against
// the hub. The mock returns 404, so FromStore fails; we assert the preflight
// surfaced that as an autoinstall error for the right coordinate (proving the
// install path fired rather than being skipped).
func TestEnsureRequestedInstalled_AttemptsInstallWhenMissing(t *testing.T) {
	binDir := t.TempDir()
	configureRun(t, true, binDir, "acme/missing", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	viper.Set("hub-url", srv.URL)

	err := ensureRequestedInstalled(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("expected an install error for a coordinate the hub returns 404 for")
	}
	if !strings.Contains(err.Error(), "installing acme/missing") {
		t.Errorf("error should name the coordinate that failed to install, got: %v", err)
	}
}

// No services configured => nothing to install, even with autoinstall on.
func TestEnsureRequestedInstalled_NoServices(t *testing.T) {
	configureRun(t, true, t.TempDir(), "", "")
	if err := ensureRequestedInstalled(context.Background(), io.Discard); err != nil {
		t.Fatalf("no services should be a no-op, got: %v", err)
	}
}

// Run nests the preflight: with autoinstall on and a missing plugin, a failed
// install aborts the run with BadUsage and the plugin loop never runs (proven by
// a getPlugins that fails the test if called).
func TestRun_PreflightFailureAbortsWithBadUsage(t *testing.T) {
	configureRun(t, true, t.TempDir(), "acme/missing", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	viper.Set("hub-url", srv.URL)

	getPlugins := func() []*PluginPkg {
		t.Error("plugin loop must not run when the preflight fails")
		return nil
	}
	if code := Run(context.Background(), flushWriter{}, hclog.NewNullLogger(), getPlugins); code != BadUsage {
		t.Errorf("expected BadUsage (%d) on preflight failure, got %d", BadUsage, code)
	}
}

// With autoinstall off, Run skips the preflight and delegates to the plugin loop;
// an empty plugin set yields NoTests, confirming the loop was reached.
func TestRun_DisabledPreflightDelegatesToLoop(t *testing.T) {
	configureRun(t, false, t.TempDir(), "", "")
	if code := Run(context.Background(), flushWriter{}, hclog.NewNullLogger(), func() []*PluginPkg { return nil }); code != NoTests {
		t.Errorf("expected NoTests (%d) from an empty run, got %d", NoTests, code)
	}
}
