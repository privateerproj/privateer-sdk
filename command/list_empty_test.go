package command

import (
	"bytes"
	"strings"
	"testing"
)

// fakeWriter wraps a bytes.Buffer and satisfies the Writer interface so tests
// can capture renderInstallableList output without a real bufio.Writer.
type fakeWriter struct{ bytes.Buffer }

func (f *fakeWriter) Flush() error { return nil }

func TestRenderInstallableList_EmptyRemote(t *testing.T) {
	// Hub returned zero plugins — the user must see a helpful message, not a
	// blank list.
	var w fakeWriter
	renderInstallableList(&w, nil, nil, "https://hub.grc.store")

	out := w.String()
	if !strings.Contains(out, "Plugins that can be installed:") {
		t.Errorf("missing header; got: %q", out)
	}
	if !strings.Contains(out, "no plugins published on https://hub.grc.store yet") {
		t.Errorf("missing empty-state message; got: %q", out)
	}
}

func TestRenderInstallableList_AllAlreadyInstalled(t *testing.T) {
	// Remote list has entries but they are all locally installed — the filtered
	// output is empty, and the message must say so accurately rather than
	// claiming the hub published nothing.
	remote := []*PluginPkg{{Name: "ossf/pvtr-github-repo"}}
	local := []*PluginPkg{{Name: "ossf/pvtr-github-repo", Installed: true}}

	var w fakeWriter
	renderInstallableList(&w, remote, local, "https://hub.example.com")

	out := w.String()
	if !strings.Contains(out, "Plugins that can be installed:") {
		t.Errorf("missing header; got: %q", out)
	}
	if !strings.Contains(out, "all published plugins are already installed") {
		t.Errorf("missing all-installed message; got: %q", out)
	}
	if strings.Contains(out, "no plugins published") {
		t.Errorf("should not claim the hub is empty when plugins exist; got: %q", out)
	}
	// The already-installed plugin must not appear as an installable entry.
	if strings.Contains(out, "ossf/pvtr-github-repo") {
		t.Errorf("installed plugin should not appear in installable list; got: %q", out)
	}
}

func TestRenderInstallableList_NonEmpty(t *testing.T) {
	// When there are plugins not yet installed, they should be listed and the
	// empty-state message must NOT appear.
	remote := []*PluginPkg{
		{Name: "ossf/pvtr-github-repo"},
		{Name: "finos/pvtr-ccc"},
	}
	local := []*PluginPkg{{Name: "ossf/pvtr-github-repo", Installed: true}}

	var w fakeWriter
	renderInstallableList(&w, remote, local, "https://hub.grc.store")

	out := w.String()
	if !strings.Contains(out, "Plugins that can be installed:") {
		t.Errorf("missing header; got: %q", out)
	}
	if !strings.Contains(out, "finos/pvtr-ccc") {
		t.Errorf("expected uninstalled plugin in list; got: %q", out)
	}
	if strings.Contains(out, "ossf/pvtr-github-repo") {
		t.Errorf("already-installed plugin should not appear; got: %q", out)
	}
	if strings.Contains(out, "no plugins published") {
		t.Errorf("empty-state message should not appear when list is non-empty; got: %q", out)
	}
}
