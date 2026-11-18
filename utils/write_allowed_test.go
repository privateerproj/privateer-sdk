package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteAllowed_Success verifies that WriteAllowed returns nil for a writable path
// and closes the file descriptor so subsequent operations do not fail.
func TestWriteAllowed_Success(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "writable.txt")

	if err := WriteAllowed(target); err != nil {
		t.Fatalf("WriteAllowed(%q) unexpected error: %v", target, err)
	}

	// Verify the file was closed by removing it (on Windows, open handles block removal).
	if err := os.Remove(target); err != nil {
		t.Errorf("expected file to be closed and removable, got: %v", err)
	}
}

// TestWriteAllowed_PermissionDenied verifies that permission denial returns an error
// properly formatted with the path and without format specifier artifacts (%!(EXTRA...)).
func TestWriteAllowed_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission-bit test on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "readonly.txt")

	if err := os.WriteFile(target, []byte("data"), 0400); err != nil {
		t.Fatalf("creating read-only file: %v", err)
	}

	err := WriteAllowed(target)
	if err == nil {
		t.Skip("skipping if running as root with bypass permissions")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "[ERROR] Permissions prevent this from writing to file: ") {
		t.Errorf("expected permission error prefix, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, target) {
		t.Errorf("expected error to include target path %q, got: %q", target, errMsg)
	}
	if strings.Contains(errMsg, "%!(EXTRA") {
		t.Errorf("error message contains unformatted extra arguments: %q", errMsg)
	}
}

// TestWriteAllowed_NonexistentDirectory verifies that attempting to write to a path
// in a nonexistent parent directory returns a properly formatted error.
func TestWriteAllowed_NonexistentDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing-parent", "file.txt")

	err := WriteAllowed(target)
	if err == nil {
		t.Fatalf("expected error for nonexistent directory, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "[ERROR] Could not create or write to file: ") {
		t.Errorf("expected error prefix, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, target) {
		t.Errorf("expected error to include target path %q, got: %q", target, errMsg)
	}
	if strings.Contains(errMsg, "%!(EXTRA") || strings.Contains(errMsg, "%!(NOVERB)") {
		t.Errorf("error message contains format artifacts: %q", errMsg)
	}
}
