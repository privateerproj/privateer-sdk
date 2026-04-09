package command

import (
	"fmt"
	"log"
	"strings"

	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/privateerproj/privateer-sdk/internal/registry"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// GetInstallCmd returns the install command that can be added to a root command.
func GetInstallCmd(writer Writer) *cobra.Command {
	installCmd := &cobra.Command{
		Use:   "install [plugin-name]",
		Short: "Install a vetted plugin from the registry.",
		Long:  "Resolve the plugin name to registry metadata, then download the plugin binary from the release URL into the binaries path.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName := args[0]
			return installPlugin(writer, pluginName)
		},
	}
	return installCmd
}

func installPlugin(writer Writer, pluginName string) error {
	client := registry.NewClient()

	// Fetch the vetted list to validate the plugin name
	vetted, err := client.GetVettedList()
	if err != nil {
		return fmt.Errorf("fetching vetted plugin list: %w", err)
	}

	if !isVetted(vetted.Plugins, pluginName) {
		return fmt.Errorf("plugin %q is not in the vetted plugin list", pluginName)
	}

	// Parse owner/repo from plugin name
	owner, repo, err := parsePluginName(pluginName)
	if err != nil {
		return err
	}

	// Fetch plugin metadata
	_, _ = fmt.Fprintf(writer, "Fetching metadata for %s/%s...\n", owner, repo)
	pluginData, err := client.GetPluginData(owner, repo)
	if err != nil {
		return fmt.Errorf("fetching plugin data: %w", err)
	}

	// Determine download URL
	downloadURL, err := resolveDownloadURL(pluginData)
	if err != nil {
		return err
	}

	destDir := viper.GetString("binaries-path")
	_, _ = fmt.Fprintf(writer, "Downloading %s to %s...\n", pluginData.Name, destDir)

	err = install.FromURL(downloadURL, destDir, pluginData.Name)
	if err != nil {
		return fmt.Errorf("installing plugin: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "Successfully installed %s\n", pluginData.Name)
	if err := writer.Flush(); err != nil {
		log.Printf("Error flushing writer: %v", err)
	}
	return nil
}

// parsePluginName splits a plugin name into owner and repo.
// Accepts formats: "owner/repo" or just "repo" (assumes "privateerproj" as owner).
func parsePluginName(name string) (owner, repo string, err error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return "privateerproj", name, nil
}

func isVetted(plugins []string, name string) bool {
	for _, p := range plugins {
		if strings.TrimSpace(p) == name {
			return true
		}
	}
	return false
}

// resolveDownloadURL determines the download URL for a plugin.
// If the plugin has a direct download URL, use that.
// Otherwise, infer from GitHub releases using the source and latest version.
func resolveDownloadURL(data *registry.PluginData) (string, error) {
	if data.Download != "" {
		return data.Download, nil
	}
	if data.Source == "" || data.Latest == "" {
		return "", fmt.Errorf("plugin %s has no download URL and no source/version to infer one from", data.Name)
	}
	base := install.InferGitHubReleaseBase(data.Source, data.Latest)
	artifact := install.InferArtifactFilename(data.Name)
	return fmt.Sprintf("%s/%s", base, artifact), nil
}
