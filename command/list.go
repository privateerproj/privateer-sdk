package command

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Writer is an interface for output operations that supports writing and flushing.
type Writer interface {
	io.Writer
	Flush() error
}

// GetListCmd returns the list command with flags registered.
// writerFn is called at command execution time, so the writer can be
// initialized lazily (e.g. in a PersistentPreRun hook).
//
// Deprecated: use harness.GetListCmd instead. This will be removed once the
// pvtr CLI migrates to the command/harness import path.
func GetListCmd(writerFn func() Writer) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Consult the Charts! List all plugins that have been installed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			writer := writerFn()
			defer func() { _ = writer.Flush() }()
			if viper.GetBool("installable") {
				// The remote list IS the point of --installable, so a hub-fetch
				// failure is the command's failure — surface it (non-zero exit),
				// don't print a line and exit 0.
				return writeInstallablePlugins(cmd.Context(), writer)
			}
			writePluginDetails(cmd.Context(), writer)
			return nil
		},
	}
	SetListCmdFlags(listCmd)
	return listCmd
}

// SetListCmdFlags registers the standard list flags on the given command.
//
// Deprecated: use harness.SetListCmdFlags instead. This will be removed once the
// pvtr CLI migrates to the command/harness import path.
func SetListCmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP("all", "a", false, "Review the Fleet! List all plugins that have been installed or requested in the current config")
	_ = viper.BindPFlag("all", cmd.PersistentFlags().Lookup("all"))

	cmd.PersistentFlags().Bool("installed", false, "List only plugins that are installed locally")
	_ = viper.BindPFlag("installed", cmd.PersistentFlags().Lookup("installed"))

	cmd.PersistentFlags().Bool("installable", false, "List plugins published to grc.store that are available to install")
	_ = viper.BindPFlag("installable", cmd.PersistentFlags().Lookup("installable"))

	cmd.MarkFlagsMutuallyExclusive("all", "installed", "installable")
}

func writeInstallablePlugins(ctx context.Context, writer Writer) error {
	remote, err := fetchInstallablePlugins(ctx)
	if err != nil {
		return fmt.Errorf("loading installable plugins: %w", err)
	}
	local := getLocalPlugins()
	renderInstallableList(writer, remote, local, oci.HubURL())
	return nil
}

// renderInstallableList writes the "Plugins that can be installed:" block.
// Extracted from writeInstallablePlugins so tests can drive it without a
// network call by supplying pre-fetched remote and local slices.
// When every remote plugin is already installed (or the remote list is empty),
// an explicit empty-state message is printed so the user knows where we looked
// rather than seeing a header with nothing beneath it.
func renderInstallableList(writer io.Writer, remote, local []*PluginPkg, hubURL string) {
	_, _ = fmt.Fprintln(writer, "Plugins that can be installed:")
	printed := 0
	for _, vp := range remote {
		if !Contains(local, vp.Name) {
			_, _ = fmt.Fprintf(writer, "  - %s\n", vp.Name)
			printed++
		}
	}
	if printed == 0 {
		// Nothing left to offer. Distinguish "the hub published nothing" (fresh
		// hub or API shape-drift) from "everything is already installed" — a
		// blank list under the header is confusing either way.
		if len(remote) == 0 {
			_, _ = fmt.Fprintf(writer, "  (no plugins published on %s yet)\n", hubURL)
		} else {
			_, _ = fmt.Fprintln(writer, "  (all published plugins are already installed)")
		}
	}
}

func writePluginDetails(ctx context.Context, writer Writer) {
	_, _ = fmt.Fprintln(writer, "| Plugin \t | Version \t | Installed \t| In Current Config \t|")
	var plugins []*PluginPkg

	if viper.GetBool("installed") {
		plugins = getLocalPlugins()
	} else if viper.GetBool("all") {
		plugins = GetPlugins()
		plugins = appendRemotePlugins(ctx, plugins)
	} else {
		plugins = GetPlugins()
	}
	for _, pluginPkg := range plugins {
		version := pluginPkg.Version
		if version == "" {
			version = "-" // unpinned (latest) or not installed
		}
		_, _ = fmt.Fprintf(writer, "| %s \t | %s \t | %t \t| %t \t|\n", pluginPkg.Name, version, pluginPkg.Installed, pluginPkg.Requested)
	}
}

// getRequestedPlugins returns the plugins requested in the config, deduplicated
// by name+version so two services pinning different versions of the same plugin
// each materialize (while two services sharing the same plugin+version collapse
// to one entry).
func getRequestedPlugins() []*PluginPkg {
	services := config.GetServices()
	seen := make(map[string]bool)
	var out []*PluginPkg
	for serviceName := range services {
		pluginName := config.GetServicePlugin(serviceName)
		version := config.GetServiceVersion(serviceName)
		dedupKey := pluginName + "@" + version
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		pluginPkg := NewPluginPkg(pluginName, version, serviceName)
		pluginPkg.Requested = true
		out = append(out, pluginPkg)
	}
	return out
}

// getLocalPlugins returns installed plugins from the manifest.
func getLocalPlugins() []*PluginPkg {
	binPath := config.GetBinariesPath()
	m, err := manifest.Load(binPath)
	if err != nil {
		return nil
	}
	var plugins []*PluginPkg
	for _, p := range m.Plugins {
		pkg := &PluginPkg{
			Name:      p.Name,
			Version:   p.Version,
			Path:      filepath.Join(binPath, p.BinaryPath),
			Installed: true,
		}
		pkg.queueCmd()
		plugins = append(plugins, pkg)
	}
	return plugins
}

// GetPlugins returns a combined list of all plugins (requested and local).
// Used by Run to determine which plugins to execute.
//
// Requested plugins already carry their resolved binary path and Installed state
// from NewPluginPkg (which consults the manifest by name+version), so they are
// taken as-is. Installed plugins not requested in the config are then appended
// for visibility in `list`.
func GetPlugins() []*PluginPkg {
	output := make([]*PluginPkg, 0)
	output = append(output, getRequestedPlugins()...)
	for _, plugin := range getLocalPlugins() {
		if !Contains(output, plugin.Name) {
			output = append(output, plugin)
		}
	}
	return output
}

// fetchInstallablePlugins lists plugins published to grc.store (the hub's
// anonymous /v1/plugins directory), keyed by their <namespace>/<plugin_id>
// coordinate. This replaced the vetted-list endpoint: it is DISCOVERY, not
// curation — install-time trust is the §6 signature verification, not presence
// in this list.
func fetchInstallablePlugins(ctx context.Context) ([]*PluginPkg, error) {
	items, err := oci.NewClient().Browse(ctx)
	if err != nil {
		return nil, fmt.Errorf("browsing grc.store plugins: %w", err)
	}
	var out []*PluginPkg
	for _, it := range items {
		if name := strings.TrimSpace(it.Coordinate()); name != "/" && name != "" {
			out = append(out, &PluginPkg{Name: name})
		}
	}
	return out, nil
}

// appendRemotePlugins appends grc.store-installable plugins not already in the slice.
func appendRemotePlugins(ctx context.Context, plugins []*PluginPkg) []*PluginPkg {
	remote, err := fetchInstallablePlugins(ctx)
	if err != nil {
		log.Printf("Warning: could not fetch remote plugin list: %v", err)
		return plugins
	}
	for _, vp := range remote {
		if !Contains(plugins, vp.Name) {
			plugins = append(plugins, &PluginPkg{Name: vp.Name, Installable: true})
		}
	}
	return plugins
}

// findPlugin returns the first plugin matching by name, or nil.
func findPlugin(slice []*PluginPkg, search string) *PluginPkg {
	for _, plugin := range slice {
		if plugin.Name == search {
			return plugin
		}
	}
	return nil
}

// Contains checks if a plugin with the given name exists in the slice.
func Contains(slice []*PluginPkg, search string) bool {
	return findPlugin(slice, search) != nil
}
