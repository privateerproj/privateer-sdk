package command

import (
	"os"
	"path/filepath"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

func TestResolveSourcePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bare file path gets file:// prepended",
			input:    "/path/to/catalog.yaml",
			expected: "file:///path/to/catalog.yaml",
		},
		// Relative paths are tested separately in TestResolveSourcePathRelative
		// since the expected value depends on the current working directory.
		{
			name:     "https URL passed through",
			input:    "https://example.com/catalog.yaml",
			expected: "https://example.com/catalog.yaml",
		},
		{
			name:     "file:// URL passed through",
			input:    "file:///path/to/catalog.yaml",
			expected: "file:///path/to/catalog.yaml",
		},
		{
			name:     "http URL passed through for fetcher to handle",
			input:    "http://example.com/catalog.yaml",
			expected: "http://example.com/catalog.yaml",
		},
		{
			name:     "other schemes passed through for fetcher to handle",
			input:    "ftp://example.com/catalog.yaml",
			expected: "ftp://example.com/catalog.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := resolveSourcePath(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

// TestResolveSourcePathRelative verifies relative paths are resolved against
// the current working directory. Without this resolution, "catalog.yaml" would
// become "file://catalog.yaml" -- which url.Parse interprets as host=catalog.yaml
// and an empty path, silently breaking the fetcher.
func TestResolveSourcePathRelative(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := resolveSourcePath("catalog.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On macOS, t.TempDir() may return a /var path that resolves to /private/var.
	// filepath.Abs uses the canonical (resolved) cwd, so compute the expected
	// value the same way rather than concatenating tmp directly.
	expectedAbs, err := filepath.Abs("catalog.yaml")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	want := "file://" + expectedAbs
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestCopyNonTemplateFile(t *testing.T) {
	tests := []struct {
		name            string
		inputContent    string
		serviceName     string
		organization    string
		expectedContent string
	}{
		{
			name:            "replaces SERVICE_NAME and ORGANIZATION placeholders",
			inputContent:    "name: __SERVICE_NAME__\norg: __ORGANIZATION__\n",
			serviceName:     "my-plugin",
			organization:    "my-org",
			expectedContent: "name: my-plugin\norg: my-org\n",
		},
		{
			name:            "no placeholders passes through unchanged",
			inputContent:    "version: 2\nbefore:\n  hooks:\n    - go mod tidy\n",
			serviceName:     "my-plugin",
			organization:    "my-org",
			expectedContent: "version: 2\nbefore:\n  hooks:\n    - go mod tidy\n",
		},
		{
			name:            "multiple occurrences of same placeholder all replaced",
			inputContent:    "a: __SERVICE_NAME__\nb: __SERVICE_NAME__\nc: __ORGANIZATION__\nd: __ORGANIZATION__\n",
			serviceName:     "svc",
			organization:    "org",
			expectedContent: "a: svc\nb: svc\nc: org\nd: org\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srcDir := t.TempDir()
			outDir := t.TempDir()

			srcPath := filepath.Join(srcDir, "testfile.yaml")
			if err := os.WriteFile(srcPath, []byte(tc.inputContent), 0644); err != nil {
				t.Fatalf("failed to write source file: %v", err)
			}

			data := CatalogData{
				ServiceName:  tc.serviceName,
				Organization: tc.organization,
			}

			err := copyNonTemplateFile(data, srcPath, "testfile.yaml", outDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(outDir, "testfile.yaml"))
			if err != nil {
				t.Fatalf("failed to read output file: %v", err)
			}

			if string(got) != tc.expectedContent {
				t.Errorf("expected %q, got %q", tc.expectedContent, string(got))
			}
		})
	}
}

func TestCopyNonTemplateFilePreservesMode(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "script.sh")
	if err := os.WriteFile(srcPath, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	data := CatalogData{ServiceName: "svc", Organization: "org"}
	err := copyNonTemplateFile(data, srcPath, "script.sh", outDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fi, err := os.Stat(filepath.Join(outDir, "script.sh"))
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}

	if fi.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %o", fi.Mode().Perm())
	}
}

func TestCopyNonTemplateFileMissingSource(t *testing.T) {
	outDir := t.TempDir()

	data := CatalogData{ServiceName: "svc", Organization: "org"}
	err := copyNonTemplateFile(data, "/nonexistent/file.yaml", "file.yaml", outDir)
	if err == nil {
		t.Fatal("expected error for missing source file, got nil")
	}
}

// TestGeneratePlugin_BadUsage verifies that each missing required flag
// produces the BadUsage exit code (4), matching the contract used by `pvtr run`.
func TestGeneratePlugin_BadUsage(t *testing.T) {
	tests := []struct {
		name         string
		sourcePath   string
		serviceName  string
		organization string
	}{
		{
			name: "missing source-path",
		},
		{
			name:       "missing service-name",
			sourcePath: "any.yaml",
		},
		{
			name:        "missing organization",
			sourcePath:  "any.yaml",
			serviceName: "svc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			if tc.sourcePath != "" {
				viper.Set("source-path", tc.sourcePath)
			}
			if tc.serviceName != "" {
				viper.Set("service-name", tc.serviceName)
			}
			if tc.organization != "" {
				viper.Set("organization", tc.organization)
			}
			viper.Set("local-templates", t.TempDir())
			viper.Set("output-dir", t.TempDir())

			got := GeneratePlugin(hclog.NewNullLogger())
			if got != BadUsage {
				t.Errorf("expected BadUsage (%d), got %d", BadUsage, got)
			}
		})
	}
}
