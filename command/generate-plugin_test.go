package command

import (
	"testing"
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
