package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestFromURL_RawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("binary-content"))
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "plugin")

	err := FromURL(server.URL+"/dl", destPath, "plugin")
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	body, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "binary-content" {
		t.Errorf("content: got %q", string(body))
	}
}

func TestFromURL_StatusCodeNotOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "plugin")

	err := FromURL(server.URL+"/dl", destPath, "plugin")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if _, err := os.Stat(destPath); err == nil {
		t.Error("dest file should not be created on error")
	}
}

func TestFromURL_TarGz(t *testing.T) {
	binaryName := "pvtr-plugin"
	if runtime.GOOS == "windows" {
		binaryName = "pvtr-plugin.exe"
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte("tar-content")
	_ = tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0755, Size: int64(len(content))})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gw.Close()
	tarGzBytes := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(tarGzBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarGzBytes)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "pvtr-plugin")

	err := FromURL(server.URL+"/dl.tar.gz", destPath, "pvtr-plugin")
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	body, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "tar-content" {
		t.Errorf("content: got %q", string(body))
	}
}

func TestFromURL_Zip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("pvtr-plugin")
	_, _ = w.Write([]byte("zip-content"))
	_ = zw.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "pvtr-plugin")

	err := FromURL(server.URL+"/dl.zip", destPath, "pvtr-plugin")
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	body, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "zip-content" {
		t.Errorf("content: got %q", string(body))
	}
}

func TestFromURL_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// no body
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "plugin")

	err := FromURL(server.URL+"/dl", destPath, "plugin")
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestWriteRaw(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "out")
	err := writeRaw(bytes.NewReader([]byte("data")), destPath)
	if err != nil {
		t.Fatalf("writeRaw: %v", err)
	}
	got, _ := os.ReadFile(destPath)
	if string(got) != "data" {
		t.Errorf("content: got %q", string(got))
	}
}

func TestWriteRaw_EmptyReader(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "out")
	err := writeRaw(bytes.NewReader(nil), destPath)
	if err == nil {
		t.Fatal("expected error for empty reader")
	}
}

// Test extractTarGz with a reader that has no matching binary (error path)
func TestExtractTarGz_NoMatch(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "other-file", Mode: 0644, Size: 0})
	_ = tw.Close()
	_ = gw.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "missing")
	err := extractTarGz(&buf, destPath, "nonexistent-binary")
	if err == nil {
		t.Fatal("expected error when binary not in archive")
	}
}

// Test extractZip with single file (fallback when name doesn't match)
func TestExtractZip_SingleFileFallback(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("different-name")
	_, _ = w.Write([]byte("single"))
	_ = zw.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "out")
	err := extractZip(&buf, destPath, "requested-name")
	if err != nil {
		t.Fatalf("extractZip single-file fallback: %v", err)
	}
	got, _ := os.ReadFile(destPath)
	if string(got) != "single" {
		t.Errorf("content: got %q", string(got))
	}
}

// Test extractZip with no matching file and multiple files (error)
func TestExtractZip_NoMatch(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, _ = zw.Create("file1")
	_, _ = zw.Create("file2")
	_ = zw.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "out")
	err := extractZip(&buf, destPath, "nonexistent")
	if err == nil {
		t.Fatal("expected error when binary not in zip")
	}
}
