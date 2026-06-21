package command

import (
	"os/exec"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/privateerproj/privateer-sdk/shared"
)

func TestMergeExitCode(t *testing.T) {
	tests := []struct {
		name       string
		prev, next int
		want       int
	}{
		{"TestPass over TestPass keeps TestPass", TestPass, TestPass, TestPass},
		{"TestFail beats TestPass", TestPass, TestFail, TestFail},
		{"TestPass after TestFail keeps TestFail", TestFail, TestPass, TestFail},
		{"BadUsage beats TestFail", TestFail, BadUsage, BadUsage},
		{"TestFail does not downgrade BadUsage", BadUsage, TestFail, BadUsage},
		{"InternalError beats BadUsage", BadUsage, InternalError, InternalError},
		{"BadUsage does not downgrade InternalError", InternalError, BadUsage, InternalError},
		{"InternalError beats TestFail", TestFail, InternalError, InternalError},
		{"InternalError beats TestPass", TestPass, InternalError, InternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeExitCode(tt.prev, tt.next); got != tt.want {
				t.Errorf("mergeExitCode(%d, %d) = %d, want %d", tt.prev, tt.next, got, tt.want)
			}
		})
	}
}

// --- test fakes for pluginClient chain ---

type fakePlugin struct {
	exitCode int
	err      error
	started  bool
}

func (f *fakePlugin) Start() (int, error) {
	f.started = true
	return f.exitCode, f.err
}

type fakeClientProtocol struct {
	plugin *fakePlugin
}

func (f *fakeClientProtocol) Dispense(string) (interface{}, error) {
	return f.plugin, nil
}

func (f *fakeClientProtocol) Ping() error { return nil }
func (f *fakeClientProtocol) Close() error { return nil }

type fakePluginClient struct {
	proto  *fakeClientProtocol
	killed bool
}

func (f *fakePluginClient) Client() (hcplugin.ClientProtocol, error) {
	return f.proto, nil
}

func (f *fakePluginClient) Kill() { f.killed = true }

// installFakeClient replaces newClientFn with a factory that returns a
// fakePluginClient backed by the given fakePlugin, and returns a cleanup
// function that restores the original.
func installFakeClient(fp *fakePlugin) func() {
	orig := newClientFn
	newClientFn = func(_ *exec.Cmd, _ hclog.Logger) pluginClient {
		return &fakePluginClient{
			proto: &fakeClientProtocol{plugin: fp},
		}
	}
	return func() { newClientFn = orig }
}

func testLogger() hclog.Logger {
	return hclog.NewNullLogger()
}

// --- Run tests ---

func TestRun_SkipsNonRequestedPlugins(t *testing.T) {
	fp := &fakePlugin{exitCode: shared.TestPass}
	cleanup := installFakeClient(fp)
	defer cleanup()

	exit := Run(testLogger(), func() []*PluginPkg {
		return []*PluginPkg{
			{
				Name:          "acme/installed-only",
				Installed:     true,
				Requested:     false,
				ServiceTarget: "",
				Command:       exec.Command("unused"),
			},
		}
	})

	if fp.started {
		t.Error("non-requested plugin should not have been started")
	}
	if exit != TestPass {
		t.Errorf("expected TestPass (%d) when only non-requested plugins exist, got %d", TestPass, exit)
	}
}

func TestRun_ExecutesRequestedInstalledPlugin(t *testing.T) {
	fp := &fakePlugin{exitCode: shared.TestPass}
	cleanup := installFakeClient(fp)
	defer cleanup()

	exit := Run(testLogger(), func() []*PluginPkg {
		return []*PluginPkg{
			{
				Name:          "acme/scanner",
				Installed:     true,
				Requested:     true,
				ServiceTarget: "my-service",
				Command:       exec.Command("unused"),
			},
		}
	})

	if !fp.started {
		t.Error("requested+installed plugin should have been started")
	}
	if exit != TestPass {
		t.Errorf("expected TestPass (%d), got %d", TestPass, exit)
	}
}

func TestRun_RequestedNotInstalledReturnsBadUsage(t *testing.T) {
	fp := &fakePlugin{exitCode: shared.TestPass}
	cleanup := installFakeClient(fp)
	defer cleanup()

	exit := Run(testLogger(), func() []*PluginPkg {
		return []*PluginPkg{
			{
				Name:          "acme/missing",
				Installed:     false,
				Requested:     true,
				ServiceTarget: "svc",
				Command:       exec.Command("unused"),
			},
		}
	})

	if fp.started {
		t.Error("uninstalled plugin should not have been started")
	}
	if exit != BadUsage {
		t.Errorf("expected BadUsage (%d), got %d", BadUsage, exit)
	}
}

func TestRun_MixedRequestedAndNonRequested(t *testing.T) {
	fp := &fakePlugin{exitCode: shared.TestPass}
	cleanup := installFakeClient(fp)
	defer cleanup()

	exit := Run(testLogger(), func() []*PluginPkg {
		return []*PluginPkg{
			{
				Name:          "acme/local-only",
				Installed:     true,
				Requested:     false,
				ServiceTarget: "",
				Command:       exec.Command("unused"),
			},
			{
				Name:          "acme/scanner",
				Installed:     true,
				Requested:     true,
				ServiceTarget: "my-service",
				Command:       exec.Command("unused"),
			},
		}
	})

	if !fp.started {
		t.Error("requested plugin should have been started")
	}
	if exit != TestPass {
		t.Errorf("expected TestPass (%d), got %d", TestPass, exit)
	}
}

func TestRun_EmptyPluginListReturnsNoTests(t *testing.T) {
	exit := Run(testLogger(), func() []*PluginPkg {
		return nil
	})

	if exit != NoTests {
		t.Errorf("expected NoTests (%d), got %d", NoTests, exit)
	}
}
