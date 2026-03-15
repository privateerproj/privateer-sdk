package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// PluginMetadata is the subset of registry plugin data needed for install.
type PluginMetadata struct {
	Name              string
	Image             string
	BinaryPathInImage string
	DownloadURL       string
}

// GetInstallCmd returns the install command. Caller provides getPluginData (e.g. from registry)
// and installFromURL (direct HTTP GET of the plugin binary). Plugin metadata must include a
// "download" URL; no OCI or container runtime is required. writer is used for success/error output.
func GetInstallCmd(
	writer Writer,
	getPluginData func(name string) (*PluginMetadata, error),
	installFromURL func(downloadURL, destPath, binaryName string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   "install [plugin-name]",
		Short: "Install a vetted plugin from the registry.",
		Long:  "Resolve the plugin name to registry metadata, then download the plugin binary from the release URL into the binaries path.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nameArg := strings.TrimSpace(args[0])
			if nameArg == "" {
				return fmt.Errorf("plugin name is required")
			}
			if !strings.Contains(nameArg, "/") || strings.Contains(nameArg, "..") {
				return fmt.Errorf("plugin name must be in the form owner/repo (e.g. ossf/pvtr-github-repo-scanner)")
			}

			pd, err := getPluginData(nameArg)
			if err != nil {
				return fmt.Errorf("plugin %q: %w", nameArg, err)
			}

			downloadURL := strings.TrimSpace(pd.DownloadURL)
			if downloadURL == "" {
				return fmt.Errorf("plugin %q has no download URL; add a \"download\" field to the plugin metadata", nameArg)
			}

			binariesPath := viper.GetString("binaries-path")
			binaryName := filepath.Base(nameArg)
			if binaryName == "" || binaryName == "." {
				binaryName = strings.ReplaceAll(nameArg, "/", "-")
			}
			destPath := filepath.Join(binariesPath, binaryName)

			if err := os.MkdirAll(binariesPath, 0755); err != nil {
				return fmt.Errorf("create binaries directory %s: %w", binariesPath, err)
			}

			if err := installFromURL(downloadURL, destPath, binaryName); err != nil {
				return fmt.Errorf("install from URL %q: %w", downloadURL, err)
			}

			_, _ = fmt.Fprintf(writer, "Installed %s to %s\n", binaryName, destPath)
			return writer.Flush()
		},
	}
}
