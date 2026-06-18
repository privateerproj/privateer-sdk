package command

import "testing"

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
