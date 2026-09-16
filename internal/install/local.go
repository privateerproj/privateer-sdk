package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/catalog"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Local installs a plugin from a local binary path: it copies the binary into
// the SDK's binaries dir (atomically), registers it in the manifest as
// local/<name> at version "local", then caches the grc.store catalogs it
// declares. A local binary has no signed config to read those from, so it is
// asked directly via its publish-manifest subcommand — it is the user's own
// build, the same trust `pvtr publish` extends. Progress is written to w; the
// caller owns flushing w.
func Local(ctx context.Context, w io.Writer, binaryPath string) error {
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
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Installed local plugin %s\n", binaryName)

	m, err := RunPublishManifest(ctx, filepath.Join(destDir, binaryName))
	if err != nil {
		// Any failure to ASK the plugin is non-fatal: it may predate the
		// publish-manifest command (older SDK, embeds its catalogs and needs
		// nothing cached), or it may have timed out, crashed, or printed
		// unparseable JSON. We cannot tell which, and the binary is installed and
		// usable either way, so report and move on rather than undoing the
		// install. Not knowing which catalogs a plugin wants is different from
		// knowing and being unable to get one: the catalog install below is not
		// best-effort, and a catalog the plugin names but the hub does not have
		// fails this command.
		_, _ = fmt.Fprintf(w, "Warning: could not read the plugin's catalogs (%v); skipping catalog install\n", err)
		return nil
	}
	coords := make([]pluginkit.CatalogCoordinate, 0, len(m.Catalogs))
	for _, raw := range m.Catalogs {
		c, err := pluginkit.ParseCatalogCoordinate(raw)
		if err != nil {
			return fmt.Errorf("plugin declares an invalid catalog: %w", err)
		}
		coords = append(coords, c)
	}
	// skipUnpublished is false: these coordinates come only from AddCatalogs, so
	// each one names a catalog that is meant to be on grc.store. Skipping a
	// missing one would exit 0 here and fail every later `pvtr run`.
	return catalog.Install(ctx, w, oci.NewClient(), binDirPath, coords, false)
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
