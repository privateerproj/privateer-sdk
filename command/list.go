package command

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
	"github.com/privateerproj/privateer-sdk/internal/registry"
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
func GetListCmd(writerFn func() Writer) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Consult the Charts! List all plugins that have been installed.",
		Run: func(cmd *cobra.Command, args []string) {
			writer := writerFn()
			if viper.GetBool("installable") {
				writeInstallablePlugins(writer)
			} else {
				writePluginDetails(writer)
			}
			err := writer.Flush()
			if err != nil {
				log.Printf("Error flushing writer: %v", err)
			}
		},
	}
	SetListCmdFlags(listCmd)
	return listCmd
}

// SetListCmdFlags registers the standard list flags on the given command.
func SetListCmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP("all", "a", false, "Review the Fleet! List all plugins that have been installed or requested in the current config")
	_ = viper.BindPFlag("all", cmd.PersistentFlags().Lookup("all"))

	cmd.PersistentFlags().Bool("installed", false, "List only plugins that are installed locally")
	_ = viper.BindPFlag("installed", cmd.PersistentFlags().Lookup("installed"))

	cmd.PersistentFlags().Bool("installable", false, "List vetted plugins from the registry that are available to install")
	_ = viper.BindPFlag("installable", cmd.PersistentFlags().Lookup("installable"))

	cmd.MarkFlagsMutuallyExclusive("all", "installed", "installable")
}

func writeInstallablePlugins(writer Writer) {
	remote, err := fetchVettedPlugins()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "Error loading vetted plugins: %v\n", err)
		return
	}
	local := getLocalPlugins()
	_, _ = fmt.Fprintln(writer, "Plugins that can be installed:")
	for _, vp := range remote {
		if !Contains(local, vp.Name) {
			_, _ = fmt.Fprintf(writer, "  - %s\n", vp.Name)
		}
	}
}

func writePluginDetails(writer Writer) {
	_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
	var plugins []*PluginPkg

	if viper.GetBool("installed") {
		plugins = getLocalPlugins()
	} else if viper.GetBool("all") {
		plugins = GetPlugins()
		plugins = appendRemotePlugins(plugins)
	} else {
		plugins = GetPlugins()
	}
	for _, pluginPkg := range plugins {
		_, _ = fmt.Fprintf(writer, "| %s \t | %t \t| %t \t|\n", pluginPkg.Name, pluginPkg.Installed, pluginPkg.Requested)
	}
}

// getRequestedPlugins returns a deduplicated list of plugin names requested in the config.
func getRequestedPlugins() []*PluginPkg {
	services := config.GetServices()
	seen := make(map[string]bool)
	var out []*PluginPkg
	for serviceName := range services {
		pluginName := config.GetServicePlugin(serviceName)
		if seen[pluginName] {
			continue
		}
		seen[pluginName] = true
		pluginPkg := NewPluginPkg(pluginName, serviceName)
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
func GetPlugins() []*PluginPkg {
	output := make([]*PluginPkg, 0)
	localPlugins := getLocalPlugins()
	for _, plugin := range getRequestedPlugins() {
		if local := findPlugin(localPlugins, plugin.Name); local != nil {
			plugin.Installed = true
			plugin.Path = local.Path
			plugin.queueCmd()
		}
		output = append(output, plugin)
	}
	for _, plugin := range localPlugins {
		if !Contains(output, plugin.Name) {
			output = append(output, plugin)
		}
	}
	return output
}

// fetchVettedPlugins returns vetted plugin names from the registry.
func fetchVettedPlugins() ([]*PluginPkg, error) {
	client := registry.NewClient()
	resp, err := client.GetVettedList()
	if err != nil {
		return nil, fmt.Errorf("fetching vetted plugins: %w", err)
	}
	if len(resp.Plugins) == 0 {
		return nil, fmt.Errorf("response has no plugins array or it is empty")
	}
	var out []*PluginPkg
	for _, name := range resp.Plugins {
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, &PluginPkg{Name: name})
		}
	}
	return out, nil
}

// appendRemotePlugins appends vetted registry plugins not already in the slice.
func appendRemotePlugins(plugins []*PluginPkg) []*PluginPkg {
	remote, err := fetchVettedPlugins()
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
