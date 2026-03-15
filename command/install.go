package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/privateerproj/privateer-sdk/internal/registry"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetInstallCmd returns the install command. It fetches plugin metadata from the registry
// (PVTR_REGISTRY_URL env or default), resolves the download URL (from "download" field or
// inferred from GitHub source+latest), and downloads the binary into the binaries path.
// writer is used for success/error output.
func GetInstallCmd(writer Writer) *cobra.Command {
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

			client := registry.NewClient()
			pd, err := client.GetPluginData(nameArg)
			if err != nil {
				return fmt.Errorf("plugin %q: %w", nameArg, err)
			}

			binaryName := filepath.Base(nameArg)
			if binaryName == "" || binaryName == "." {
				binaryName = strings.ReplaceAll(nameArg, "/", "-")
			}

			base := strings.TrimSpace(pd.DownloadURL)
			if base == "" {
				inferred, ok := install.InferGitHubReleaseBase(pd.Source, pd.Latest)
				if !ok {
					return fmt.Errorf("plugin %q has no download URL and source is not a GitHub repo; add a \"download\" field to the plugin metadata", nameArg)
				}
				base = inferred
			} else {
				base = strings.TrimSuffix(base, "/")
			}

			artifactFilename, err := install.InferArtifactFilename(binaryName)
			if err != nil {
				return fmt.Errorf("plugin %q: %w", nameArg, err)
			}
			downloadURL := base + "/" + artifactFilename

			binariesPath := viper.GetString("binaries-path")
			destPath := filepath.Join(binariesPath, binaryName)

			if err := os.MkdirAll(binariesPath, 0755); err != nil {
				return fmt.Errorf("create binaries directory %s: %w", binariesPath, err)
			}

			if err := install.FromURL(downloadURL, destPath, binaryName); err != nil {
				return fmt.Errorf("install from URL %q: %w", downloadURL, err)
			}

			_, _ = fmt.Fprintf(writer, "Installed %s to %s\n", binaryName, destPath)
			return writer.Flush()
		},
	}
}
