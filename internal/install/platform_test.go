package install

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestInferGitHubReleaseBase(t *testing.T) {
	var tests = []struct {
		name     string
		source   string
		version  string
		expected string
	}{
		{
			name:     "full URL with v prefix",
			source:   "https://github.com/ossf/pvtr-github-repo-scanner",
			version:  "v0.19.2",
			expected: "https://github.com/ossf/pvtr-github-repo-scanner/releases/download/v0.19.2",
		},
		{
			name:     "full URL without v prefix",
			source:   "https://github.com/ossf/pvtr-github-repo-scanner",
			version:  "0.19.2",
			expected: "https://github.com/ossf/pvtr-github-repo-scanner/releases/download/v0.19.2",
		},
		{
			name:     "owner/repo format with v prefix",
			source:   "ossf/pvtr-github-repo-scanner",
			version:  "v1.0.0",
			expected: "https://github.com/ossf/pvtr-github-repo-scanner/releases/download/v1.0.0",
		},
		{
			name:     "owner/repo format without v prefix",
			source:   "privateerproj/pvtr-example",
			version:  "2.3.4",
			expected: "https://github.com/privateerproj/pvtr-example/releases/download/v2.3.4",
		},
		{
			name:     "trailing slash on source URL",
			source:   "https://github.com/ossf/pvtr-github-repo-scanner/",
			version:  "v0.19.2",
			expected: "https://github.com/ossf/pvtr-github-repo-scanner/releases/download/v0.19.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferGitHubReleaseBase(tt.source, tt.version)
			if got != tt.expected {
				t.Errorf("InferGitHubReleaseBase(%q, %q)\n  got:      %s\n  expected: %s", tt.source, tt.version, got, tt.expected)
			}
		})
	}
}

func TestInferArtifactFilename(t *testing.T) {
	// Since the function uses runtime.GOOS/GOARCH, we test against the current platform
	got := InferArtifactFilename("pvtr-github-repo-scanner")

	expectedOS := strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:]
	expectedArch := runtime.GOARCH
	switch expectedArch {
	case "amd64":
		expectedArch = "x86_64"
	case "386":
		expectedArch = "i386"
	}
	if runtime.GOOS == "darwin" {
		expectedArch = "all"
	}
	expectedExt := "tar.gz"
	if runtime.GOOS == "windows" {
		expectedExt = "zip"
	}
	expected := fmt.Sprintf("pvtr-github-repo-scanner_%s_%s.%s", expectedOS, expectedArch, expectedExt)

	if got != expected {
		t.Errorf("InferArtifactFilename(\"pvtr-github-repo-scanner\")\n  got:      %s\n  expected: %s", got, expected)
	}
}

func TestInferArtifactFilename_NameVariations(t *testing.T) {
	var tests = []struct {
		name     string
		prefix   string
	}{
		{name: "simple name", prefix: "my-plugin"},
		{name: "name with dots", prefix: "plugin.v2"},
		{name: "single word", prefix: "scanner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferArtifactFilename(tt.prefix)
			if !strings.HasPrefix(got, tt.prefix+"_") {
				t.Errorf("InferArtifactFilename(%q) = %q, expected to start with %q", tt.prefix, got, tt.prefix+"_")
			}
			if runtime.GOOS == "windows" {
				if !strings.HasSuffix(got, ".zip") {
					t.Errorf("InferArtifactFilename(%q) = %q, expected .zip suffix on Windows", tt.prefix, got)
				}
			} else {
				if !strings.HasSuffix(got, ".tar.gz") {
					t.Errorf("InferArtifactFilename(%q) = %q, expected .tar.gz suffix", tt.prefix, got)
				}
			}
		})
	}
}
