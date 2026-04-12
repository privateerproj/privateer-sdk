package command

import (
	"os"
	"path/filepath"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
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
		{
			name:     "relative file path gets file:// prepended",
			input:    "catalog.yaml",
			expected: "file://catalog.yaml",
		},
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

func TestCopyNonTemplateFile(t *testing.T) {
	logger := hclog.NewNullLogger()

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

			err := copyNonTemplateFile(data, srcPath, "testfile.yaml", outDir, logger)
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
	logger := hclog.NewNullLogger()
	srcDir := t.TempDir()
	outDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "script.sh")
	if err := os.WriteFile(srcPath, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	data := CatalogData{ServiceName: "svc", Organization: "org"}
	err := copyNonTemplateFile(data, srcPath, "script.sh", outDir, logger)
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
	logger := hclog.NewNullLogger()
	outDir := t.TempDir()

	data := CatalogData{ServiceName: "svc", Organization: "org"}
	err := copyNonTemplateFile(data, "/nonexistent/file.yaml", "file.yaml", outDir, logger)
	if err == nil {
		t.Fatal("expected error for missing source file, got nil")
	}
}
