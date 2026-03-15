package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/privateerproj/privateer-sdk/internal/registry"
	"github.com/spf13/viper"
)

func TestInstallCmd_Validation(t *testing.T) {
	resetViper()
	viper.Set("binaries-path", t.TempDir())

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty name", []string{""}, "plugin name is required"},
		{"whitespace only", []string{"  "}, "plugin name is required"},
		{"no slash", []string{"foo"}, "must be in the form owner/repo"},
		{"has dotdot", []string{"owner/../repo"}, "must be in the form owner/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestInstallCmd_GetPluginDataError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer func() { _ = os.Setenv("PVTR_REGISTRY_URL", orig) }()
	_ = os.Setenv("PVTR_REGISTRY_URL", server.URL)

	resetViper()
	dir := t.TempDir()
	viper.Set("binaries-path", dir)

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)
	cmd.SetArgs([]string{"ossf/pvtr-github-repo-scanner"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func TestInstallCmd_NoDownloadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			pd := registry.PluginData{Name: "ossf/plugin", Source: "https://gitlab.com/ossf/plugin", Latest: "1.0"}
			_ = json.NewEncoder(w).Encode(pd)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer func() { _ = os.Setenv("PVTR_REGISTRY_URL", orig) }()
	_ = os.Setenv("PVTR_REGISTRY_URL", server.URL)

	resetViper()
	dir := t.TempDir()
	viper.Set("binaries-path", dir)

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)
	cmd.SetArgs([]string{"ossf/plugin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no download URL") {
		t.Errorf("error should mention no download URL: %v", err)
	}
}

func TestInstallCmd_MkdirAllError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			pd := registry.PluginData{Name: "ossf/plugin", DownloadURL: "https://example.com/dl"}
			_ = json.NewEncoder(w).Encode(pd)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer func() { _ = os.Setenv("PVTR_REGISTRY_URL", orig) }()
	_ = os.Setenv("PVTR_REGISTRY_URL", server.URL)

	resetViper()
	dir := t.TempDir()
	pathAsFile := filepath.Join(dir, "file")
	if err := os.WriteFile(pathAsFile, nil, 0644); err != nil {
		t.Fatal(err)
	}
	viper.Set("binaries-path", pathAsFile)

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)
	cmd.SetArgs([]string{"ossf/plugin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when binaries-path is a file")
	}
	if !strings.Contains(err.Error(), "create binaries directory") {
		t.Errorf("error should mention create binaries directory: %v", err)
	}
}

func TestInstallCmd_Success(t *testing.T) {
	artifactFilename, err := install.InferArtifactFilename("pvtr-github-repo-scanner")
	if err != nil {
		t.Fatal(err)
	}
	var tarGzBuf bytes.Buffer
	{
		gw := gzip.NewWriter(&tarGzBuf)
		tw := tar.NewWriter(gw)
		binName := "pvtr-github-repo-scanner"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		content := []byte("binary-content")
		_ = tw.WriteHeader(&tar.Header{Name: binName, Mode: 0755, Size: int64(len(content))})
		_, _ = tw.Write(content)
		_ = tw.Close()
		_ = gw.Close()
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			pd := registry.PluginData{
				Name:        "ossf/pvtr-github-repo-scanner",
				DownloadURL: "http://" + r.Host + "/releases",
			}
			_ = json.NewEncoder(w).Encode(pd)
			return
		}
		if strings.HasSuffix(r.URL.Path, artifactFilename) {
			w.Header().Set("Content-Length", strconv.Itoa(tarGzBuf.Len()))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(tarGzBuf.Bytes())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer func() { _ = os.Setenv("PVTR_REGISTRY_URL", orig) }()
	_ = os.Setenv("PVTR_REGISTRY_URL", server.URL)

	resetViper()
	dir := t.TempDir()
	viper.Set("binaries-path", dir)

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)
	cmd.SetArgs([]string{"ossf/pvtr-github-repo-scanner"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	destPath := filepath.Join(dir, "pvtr-github-repo-scanner")
	body, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "binary-content" {
		t.Errorf("content: got %q", string(body))
	}
	if !strings.Contains(buf.String(), "Installed") {
		t.Errorf("output should mention Installed: %s", buf.String())
	}
}

// TestInstallCmd_BinaryNameFallback: when filepath.Base(name) is ".", binaryName becomes "owner-."
func TestInstallCmd_BinaryNameFallback(t *testing.T) {
	artifactFilename, err := install.InferArtifactFilename("owner-.")
	if err != nil {
		t.Fatal(err)
	}
	var tarGzBuf bytes.Buffer
	{
		gw := gzip.NewWriter(&tarGzBuf)
		tw := tar.NewWriter(gw)
		content := []byte("x")
		_ = tw.WriteHeader(&tar.Header{Name: "owner-.", Mode: 0755, Size: int64(len(content))})
		_, _ = tw.Write(content)
		_ = tw.Close()
		_ = gw.Close()
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "owner") && strings.HasSuffix(r.URL.Path, ".json") {
			pd := registry.PluginData{
				Name:        "owner/.",
				DownloadURL: "http://" + r.Host + "/dl",
			}
			_ = json.NewEncoder(w).Encode(pd)
			return
		}
		if strings.HasSuffix(r.URL.Path, artifactFilename) {
			w.Header().Set("Content-Length", strconv.Itoa(tarGzBuf.Len()))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(tarGzBuf.Bytes())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer func() { _ = os.Setenv("PVTR_REGISTRY_URL", orig) }()
	_ = os.Setenv("PVTR_REGISTRY_URL", server.URL)

	resetViper()
	dir := t.TempDir()
	viper.Set("binaries-path", dir)

	buf := &bytes.Buffer{}
	writer := &flushWriter{Buffer: buf}
	cmd := GetInstallCmd(writer)
	cmd.SetArgs([]string{"owner/."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	destPath := filepath.Join(dir, "owner-.")
	body, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "x" {
		t.Errorf("content: got %q", string(body))
	}
}

// flushWriter wraps a bytes.Buffer and implements Writer (io.Writer + Flush).
type flushWriter struct {
	*bytes.Buffer
}

func (f *flushWriter) Flush() error { return nil }
