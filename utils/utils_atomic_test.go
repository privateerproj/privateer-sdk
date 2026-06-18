package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteFileAtomic_WritesContent verifies that WriteFileAtomic creates the
// destination file with the expected content.
func TestWriteFileAtomic_WritesContent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.bin")
	data := []byte("hello atomic world")

	if err := WriteFileAtomic(dest, data, 0644); err != nil {
		t.Fatalf("WriteFileAtomic error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}
}

// TestWriteFileAtomic_NoTmpOnSuccess verifies that the .tmp side-file is
// cleaned up on a successful write — only the destination file should exist.
func TestWriteFileAtomic_NoTmpOnSuccess(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.bin")

	if err := WriteFileAtomic(dest, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic error: %v", err)
	}

	tmp := dest + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after success, stat err: %v", tmp, err)
	}
}

// TestWriteFileAtomic_AppliesPerm verifies that the destination file is
// created with (at least) the requested permission bits. On Windows, file
// mode enforcement is limited — we skip the mode check there.
func TestWriteFileAtomic_AppliesPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix permission bits — skipping mode check")
	}
	dir := t.TempDir()

	cases := []struct {
		name string
		perm os.FileMode
	}{
		{"0644 manifest perm", 0644},
		{"0755 binary perm", 0755},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(dir, tc.name)
			if err := WriteFileAtomic(dest, []byte("x"), tc.perm); err != nil {
				t.Fatalf("WriteFileAtomic error: %v", err)
			}
			info, err := os.Stat(dest)
			if err != nil {
				t.Fatalf("stat error: %v", err)
			}
			// Compare only the permission bits (mask out sticky/setuid/setgid and
			// any umask bits that the OS applies on top of what we requested).
			got := info.Mode().Perm()
			// The written perm may be narrowed by the process umask but must not
			// have bits that are absent in tc.perm.
			if got&^tc.perm != 0 {
				t.Errorf("mode %04o has bits beyond requested %04o", got, tc.perm)
			}
			// At minimum the owner read bit must survive any reasonable umask.
			if got&0400 == 0 {
				t.Errorf("mode %04o lost owner-read bit", got)
			}
		})
	}
}

// TestWriteFileAtomic_OverwritesExisting verifies that WriteFileAtomic replaces
// an existing file atomically, leaving only the new content.
func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.bin")

	// Write an initial version.
	if err := WriteFileAtomic(dest, []byte("old content"), 0644); err != nil {
		t.Fatalf("initial WriteFileAtomic error: %v", err)
	}
	// Overwrite.
	if err := WriteFileAtomic(dest, []byte("new content"), 0644); err != nil {
		t.Fatalf("overwrite WriteFileAtomic error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("expected overwritten content, got %q", got)
	}
}
