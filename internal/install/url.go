package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FromURL downloads an artifact from the given URL and extracts the binary
// into destDir with the given binaryName. Supports .tar.gz and .zip archives,
// as well as raw binaries.
func FromURL(url, destDir, binaryName string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "pvtr/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	destPath := filepath.Join(destDir, binaryName)

	switch {
	case strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz"):
		return extractFromTarGz(body, destPath, binaryName)
	case strings.HasSuffix(url, ".zip"):
		return extractFromZip(body, destPath, binaryName)
	default:
		return writeExecutable(destPath, body)
	}
}

func extractFromTarGz(data []byte, destPath, binaryName string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("opening gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		name := filepath.Base(header.Name)
		if name == binaryName || strings.TrimSuffix(name, ".exe") == binaryName {
			content, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading %s from archive: %w", name, err)
			}
			return writeExecutable(destPath, content)
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func extractFromZip(data []byte, destPath, binaryName string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening zip reader: %w", err)
	}

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == binaryName || strings.TrimSuffix(name, ".exe") == binaryName {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("opening %s in zip: %w", name, err)
			}
			defer rc.Close()

			content, err := io.ReadAll(rc)
			if err != nil {
				return fmt.Errorf("reading %s from zip: %w", name, err)
			}
			return writeExecutable(destPath, content)
		}
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func writeExecutable(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0755); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
