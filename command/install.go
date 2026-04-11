package command

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/registry"
	"github.com/spf13/cobra"
)

var validNameSegment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

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
	defer func() { _ = writer.Flush() }()
	client := registry.NewClient()

	// Parse owner/repo from plugin name
	owner, repo, err := parsePluginName(pluginName)
	if err != nil {
		return err
	}

	// Fetch the vetted list and validate using the normalized owner/repo form
	fullName := owner + "/" + repo
	vetted, err := client.GetVettedList()
	if err != nil {
		return fmt.Errorf("fetching vetted plugin list: %w", err)
	}

	if !isVetted(vetted.Plugins, fullName) {
		return fmt.Errorf("plugin %q is not in the vetted plugin list", fullName)
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

	destDir := config.GetBinariesPath()
	binaryName := path.Base(pluginData.Name)
	if !validNameSegment.MatchString(binaryName) {
		return fmt.Errorf("invalid binary name %q from registry", binaryName)
	}
	_, _ = fmt.Fprintf(writer, "Downloading %s to %s...\n", binaryName, destDir)

	err = install.FromURL(downloadURL, destDir, binaryName)
	if err != nil {
		return fmt.Errorf("installing plugin: %w", err)
	}

	// Update the plugin manifest
	m, err := manifest.Load(destDir)
	if err != nil {
		return fmt.Errorf("loading plugin manifest: %w", err)
	}
	m.Add(manifest.Plugin{
		Name:       fullName,
		Version:    pluginData.Latest,
		BinaryPath: binaryName,
	})
	if err := m.Save(destDir); err != nil {
		return fmt.Errorf("saving plugin manifest: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "Successfully installed %s\n", pluginData.Name)
	return nil
}

// parsePluginName splits a plugin name into owner and repo.
// Accepts formats: "owner/repo" or just "repo" (assumes "privateerproj" as owner).
// Returns an error if the name is empty or contains path traversal characters.
func parsePluginName(name string) (owner, repo string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("plugin name must not be empty")
	}
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		owner, repo = parts[0], parts[1]
	} else {
		owner, repo = "privateerproj", name
	}
	if !validNameSegment.MatchString(owner) {
		return "", "", fmt.Errorf("invalid owner %q: must match %s", owner, validNameSegment.String())
	}
	if !validNameSegment.MatchString(repo) {
		return "", "", fmt.Errorf("invalid repo %q: must match %s", repo, validNameSegment.String())
	}
	return owner, repo, nil
}

func isVetted(plugins []string, name string) bool {
	name = strings.TrimSpace(name)
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
	binaryName := path.Base(data.Name)
	artifact := install.InferArtifactFilename(binaryName)
	return fmt.Sprintf("%s/%s", base, artifact), nil
}
