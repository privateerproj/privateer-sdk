package command

import (
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/registry"
)

func TestParsePluginName(t *testing.T) {
	var tests = []struct {
		name          string
		input         string
		expectedOwner string
		expectedRepo  string
	}{
		{
			name:          "owner/repo format",
			input:         "ossf/pvtr-github-repo-scanner",
			expectedOwner: "ossf",
			expectedRepo:  "pvtr-github-repo-scanner",
		},
		{
			name:          "repo only defaults to privateerproj",
			input:         "pvtr-example",
			expectedOwner: "privateerproj",
			expectedRepo:  "pvtr-example",
		},
		{
			name:          "org with nested path",
			input:         "myorg/my-plugin",
			expectedOwner: "myorg",
			expectedRepo:  "my-plugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parsePluginName(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.expectedOwner {
				t.Errorf("owner: got %q, expected %q", owner, tt.expectedOwner)
			}
			if repo != tt.expectedRepo {
				t.Errorf("repo: got %q, expected %q", repo, tt.expectedRepo)
			}
		})
	}
}

func TestIsVetted(t *testing.T) {
	plugins := []string{"ossf/pvtr-github-repo-scanner", "privateerproj/pvtr-example", " spaced-name "}

	var tests = []struct {
		name     string
		search   string
		expected bool
	}{
		{name: "exact match", search: "ossf/pvtr-github-repo-scanner", expected: true},
		{name: "another match", search: "privateerproj/pvtr-example", expected: true},
		{name: "not in list", search: "unknown/plugin", expected: false},
		{name: "empty string", search: "", expected: false},
		{name: "trimmed match", search: "spaced-name", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVetted(plugins, tt.search)
			if got != tt.expected {
				t.Errorf("isVetted(plugins, %q) = %v, expected %v", tt.search, got, tt.expected)
			}
		})
	}
}

func TestResolveDownloadURL(t *testing.T) {
	var tests = []struct {
		name        string
		data        *registry.PluginData
		expectURL   string
		expectError string
	}{
		{
			name: "direct download URL",
			data: &registry.PluginData{
				Name:     "ossf/pvtr-github-repo-scanner",
				Download: "https://example.com/direct-download.tar.gz",
				Source:   "https://github.com/ossf/pvtr-github-repo-scanner",
				Latest:   "0.19.2",
			},
			expectURL: "https://example.com/direct-download.tar.gz",
		},
		{
			name: "inferred from GitHub release",
			data: &registry.PluginData{
				Name:   "ossf/pvtr-github-repo-scanner",
				Source: "https://github.com/ossf/pvtr-github-repo-scanner",
				Latest: "0.19.2",
			},
			expectURL: "https://github.com/ossf/pvtr-github-repo-scanner/releases/download/v0.19.2/pvtr-github-repo-scanner_",
		},
		{
			name: "inferred from owner/repo source",
			data: &registry.PluginData{
				Name:   "privateerproj/pvtr-example",
				Source: "privateerproj/pvtr-example",
				Latest: "1.0.0",
			},
			expectURL: "https://github.com/privateerproj/pvtr-example/releases/download/v1.0.0/pvtr-example_",
		},
		{
			name: "missing source and download",
			data: &registry.PluginData{
				Name:   "broken-plugin",
				Latest: "1.0.0",
			},
			expectError: "no download URL and no source/version",
		},
		{
			name: "missing version",
			data: &registry.PluginData{
				Name:   "broken-plugin",
				Source: "https://github.com/org/repo",
			},
			expectError: "no download URL and no source/version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDownloadURL(tt.data)

			if tt.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectError)
				}
				if !strings.Contains(err.Error(), tt.expectError) {
					t.Errorf("expected error containing %q, got: %v", tt.expectError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// For inferred URLs, we only check the prefix since the suffix depends on OS/arch
			if tt.data.Download != "" {
				if got != tt.expectURL {
					t.Errorf("got %q, expected %q", got, tt.expectURL)
				}
			} else {
				if !strings.HasPrefix(got, tt.expectURL) {
					t.Errorf("got %q, expected prefix %q", got, tt.expectURL)
				}
			}
		})
	}
}

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
