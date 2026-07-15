package command

import (
	"strings"
	"testing"
)

func TestContains(t *testing.T) {
	plugins := []*PluginPkg{
		{Name: "plugin-a"},
		{Name: "plugin-b"},
		{Name: "pvtr-github-repo-scanner"},
	}

	var tests = []struct {
		name     string
		search   string
		expected bool
	}{
		{name: "found first", search: "plugin-a", expected: true},
		{name: "found last", search: "pvtr-github-repo-scanner", expected: true},
		{name: "not found", search: "missing-plugin", expected: false},
		{name: "empty string", search: "", expected: false},
		{name: "nil slice", search: "anything", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slice := plugins
			if tt.name == "nil slice" {
				slice = nil
			}
			got := Contains(slice, tt.search)
			if got != tt.expected {
				t.Errorf("Contains(plugins, %q) = %v, expected %v", tt.search, got, tt.expected)
			}
		})
	}
}

func TestRenderPluginDetails(t *testing.T) {
	countRows := func(out, name, version string) int {
		row := "| " + name + " \t | " + version + " \t "
		return strings.Count(out, row)
	}

	t.Run("dedupes two services sharing plugin and version", func(t *testing.T) {
		// getRequestedPlugins emits one entry per service, so a config with two
		// services on the same plugin+version yields duplicate PluginPkgs. The
		// display must collapse them to a single row.
		plugins := []*PluginPkg{
			{Name: "acme/scanner", Version: "1.0.0", Installed: true, Requested: true},
			{Name: "acme/scanner", Version: "1.0.0", Installed: true, Requested: true},
		}
		var b strings.Builder
		renderPluginDetails(&b, plugins)
		if n := countRows(b.String(), "acme/scanner", "1.0.0"); n != 1 {
			t.Errorf("expected 1 row for acme/scanner@1.0.0, got %d\n%s", n, b.String())
		}
	})

	t.Run("keeps distinct versions of same plugin", func(t *testing.T) {
		plugins := []*PluginPkg{
			{Name: "acme/scanner", Version: "1.0.0", Installed: true, Requested: true},
			{Name: "acme/scanner", Version: "2.0.0", Installed: true, Requested: true},
		}
		var b strings.Builder
		renderPluginDetails(&b, plugins)
		if n := countRows(b.String(), "acme/scanner", "1.0.0"); n != 1 {
			t.Errorf("expected 1 row for acme/scanner@1.0.0, got %d", n)
		}
		if n := countRows(b.String(), "acme/scanner", "2.0.0"); n != 1 {
			t.Errorf("expected 1 row for acme/scanner@2.0.0, got %d", n)
		}
	})

	t.Run("renders empty version as dash", func(t *testing.T) {
		plugins := []*PluginPkg{{Name: "acme/unpinned", Version: "", Requested: true}}
		var b strings.Builder
		renderPluginDetails(&b, plugins)
		if n := countRows(b.String(), "acme/unpinned", "-"); n != 1 {
			t.Errorf("expected unpinned plugin to render version as '-', got:\n%s", b.String())
		}
	})
}
