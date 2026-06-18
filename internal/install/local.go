package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Local installs a plugin from a local binary path: it copies the binary into
// the SDK's binaries dir (atomically) and registers it in the manifest as
// local/<name> at version "local". Progress is written to w; the caller owns
// flushing w.
func Local(w io.Writer, binaryPath string) error {
	binaryName, err := getSourceName(binaryPath)
	if err != nil {
		return err
	}

	binDirPath := config.GetBinariesPath()
	destDir := filepath.Join(binDirPath, "local")

	err = os.MkdirAll(destDir, 0o755)
	if err != nil {
		return fmt.Errorf("creating local plugin directory: %w", err)
	}

	err = moveFileWithCrashProtection(binaryPath, destDir, binaryName)
	if err != nil {
		return err
	}

	err = saveManifestInMem(binDirPath, binaryName)
	if err == nil {
		_, _ = fmt.Fprintf(w, "Installed local plugin %s\n", binaryName)
	}
	return err
}

func getSourceName(binaryPath string) (string, error) {
	info, err := os.Stat(binaryPath)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", binaryPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a binary", binaryPath)
	}

	binaryName := filepath.Base(binaryPath)
	if !validNameSegmentRegex.MatchString(binaryName) {
		return "", fmt.Errorf("invalid binary name %q", binaryName)
	}
	return binaryName, err
}

// Read file and then write it to the new location with atomic write
// (temp + rename) so a crash mid-copy can't leave a partial binary
// that would be detected as if it were a complete plugin.
func moveFileWithCrashProtection(binaryPath, destDir, binaryName string) error {
	src, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", binaryPath, err)
	}

	destPath := filepath.Join(destDir, binaryName)
	err = utils.WriteFileAtomic(destPath, src, 0o755)
	if err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}
	return nil
}

func saveManifestInMem(binDirPath, binaryName string) error {
	manifestBinaryPath := filepath.Join("local", binaryName)
	if err := manifest.Update(binDirPath, func(m *manifest.Manifest) error {
		m.Add(manifest.Plugin{
			Name:       "local/" + binaryName,
			Version:    "local",
			BinaryPath: manifestBinaryPath,
		})
		return nil
	}); err != nil {
		return fmt.Errorf("saving plugin manifest: %w", err)
	}
	return nil
}
