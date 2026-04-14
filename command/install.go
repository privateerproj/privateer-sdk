package command

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
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
	var localPath string

	installCmd := &cobra.Command{
		Use:   "install [plugin-name]",
		Short: "Install a plugin from the registry or a local path.",
		Long:  "Install a vetted plugin from the registry, or use --local to install a plugin binary from a local path.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if localPath != "" {
				return installLocal(writer, localPath)
			}
			if len(args) == 0 {
				return fmt.Errorf("plugin name is required (or use --local)")
			}
			return installPlugin(writer, args[0])
		},
	}
	installCmd.Flags().StringVar(&localPath, "local", "", "Path to a local plugin binary to install")
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
	if runtime.GOOS == "windows" && !strings.HasSuffix(binaryName, ".exe") {
		binaryName = binaryName + ".exe"
	}
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

func installLocal(writer Writer, binaryPath string) error {
	defer func() { _ = writer.Flush() }()

	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", binaryPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a binary", binaryPath)
	}

	binaryName := filepath.Base(binaryPath)
	if !validNameSegment.MatchString(binaryName) {
		return fmt.Errorf("invalid binary name %q", binaryName)
	}

	binPath := config.GetBinariesPath()
	destDir := filepath.Join(binPath, "local")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating local plugin directory: %w", err)
	}

	src, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", binaryPath, err)
	}
	destPath := filepath.Join(destDir, binaryName)
	if err := os.WriteFile(destPath, src, 0755); err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}

	m, err := manifest.Load(binPath)
	if err != nil {
		return fmt.Errorf("loading plugin manifest: %w", err)
	}
	manifestBinaryPath := filepath.Join("local", binaryName)
	m.Add(manifest.Plugin{
		Name:       "local/" + binaryName,
		Version:    "local",
		BinaryPath: manifestBinaryPath,
	})
	if err := m.Save(binPath); err != nil {
		return fmt.Errorf("saving plugin manifest: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "Installed local plugin %s\n", binaryName)
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
