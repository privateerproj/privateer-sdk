package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFromURL_RawBinary(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho hello")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binaryContent)
	}))
	defer server.Close()

	destDir := t.TempDir()
	err := FromURL(server.URL+"/my-plugin", destDir, "my-plugin")
	if err != nil {
		t.Fatalf("FromURL returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "my-plugin"))
	if err != nil {
		t.Fatalf("reading installed binary: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Errorf("binary content mismatch:\n  got:      %q\n  expected: %q", got, binaryContent)
	}

	info, _ := os.Stat(filepath.Join(destDir, "my-plugin"))
	if info.Mode().Perm()&0100 == 0 {
		t.Error("installed binary is not executable")
	}
}

func TestFromURL_TarGz(t *testing.T) {
	binaryContent := []byte("the-binary-content")
	archive := createTarGz(t, "my-plugin", binaryContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destDir := t.TempDir()
	err := FromURL(server.URL+"/my-plugin.tar.gz", destDir, "my-plugin")
	if err != nil {
		t.Fatalf("FromURL returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "my-plugin"))
	if err != nil {
		t.Fatalf("reading installed binary: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Errorf("binary content mismatch:\n  got:      %q\n  expected: %q", got, binaryContent)
	}
}

func TestFromURL_TarGz_BinaryNotFound(t *testing.T) {
	archive := createTarGz(t, "wrong-name", []byte("content"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	destDir := t.TempDir()
	err := FromURL(server.URL+"/archive.tar.gz", destDir, "my-plugin")
	if err == nil {
		t.Fatal("expected error when binary not found in archive, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("not found in archive")) {
		t.Errorf("expected 'not found in archive' error, got: %v", err)
	}
}

func TestFromURL_HTTPError(t *testing.T) {
	var tests = []struct {
		name       string
		statusCode int
	}{
		{name: "404 Not Found", statusCode: http.StatusNotFound},
		{name: "500 Internal Server Error", statusCode: http.StatusInternalServerError},
		{name: "403 Forbidden", statusCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			destDir := t.TempDir()
			err := FromURL(server.URL+"/plugin.tar.gz", destDir, "plugin")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tt.statusCode)
			}
			expected := fmt.Sprintf("status %d", tt.statusCode)
			if !bytes.Contains([]byte(err.Error()), []byte(expected)) {
				t.Errorf("expected error containing %q, got: %v", expected, err)
			}
		})
	}
}

func TestFromURL_CreatesDestDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()

	destDir := filepath.Join(t.TempDir(), "nested", "dir")
	err := FromURL(server.URL+"/plugin", destDir, "plugin")
	if err != nil {
		t.Fatalf("FromURL returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "plugin")); err != nil {
		t.Errorf("expected binary at %s, got error: %v", filepath.Join(destDir, "plugin"), err)
	}
}

// createTarGz creates an in-memory tar.gz archive containing a single file.
func createTarGz(t *testing.T, filename string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	err := tw.WriteHeader(&tar.Header{
		Name: filename,
		Size: int64(len(content)),
		Mode: 0755,
	})
	if err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	return buf.Bytes()
}
