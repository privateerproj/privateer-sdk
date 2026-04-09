package command

import (
	"fmt"
	"io"
	"log"
	"path"
	"strings"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/registry"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Writer is an interface for output operations that supports writing and flushing.
type Writer interface {
	io.Writer
	Flush() error
}

// GetListCmd returns the list command that can be added to a root command.
func GetListCmd(writer Writer) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Consult the Charts! List all plugins that have been installed.",
		Run: func(cmd *cobra.Command, args []string) {
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
	return listCmd
}

func writeInstallablePlugins(writer Writer) {
	plugins, err := getInstallablePlugins()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "Error loading vetted plugins: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(writer, "Plugins that can be installed:")
	for _, pluginPkg := range plugins {
		_, _ = fmt.Fprintf(writer, "  - %s\n", pluginPkg.Name)
	}
}

func writePluginDetails(writer Writer) {
	_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
	var plugins []*PluginPkg

	if viper.GetBool("installed") {
		plugins = getLocalPlugins()
	} else if viper.GetBool("all") {
		plugins = getLocalAndRemotePlugins()
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
	seen := make(map[string]*PluginPkg)
	var requestedPluginPackages []*PluginPkg
	for serviceName := range services {
		pluginName := config.GetServicePlugin(serviceName)
		if _, exists := seen[pluginName]; exists {
			continue
		}
		pluginPkg := NewPluginPkg(pluginName, serviceName)
		pluginPkg.Requested = true
		seen[pluginName] = pluginPkg
		requestedPluginPackages = append(requestedPluginPackages, pluginPkg)
	}
	return requestedPluginPackages
}

// getLocalPlugins returns a list of plugins found in the binaries path.
func getLocalPlugins() []*PluginPkg {
	var installedPlugins []*PluginPkg
	pluginPaths, _ := hcplugin.Discover("*", viper.GetString("binaries-path"))
	for _, pluginPath := range pluginPaths {
		name := path.Base(pluginPath)
		if strings.Contains(name, "privateer") {
			continue
		}
		pluginPkg := NewPluginPkg(name, "")
		pluginPkg.Installed = true
		installedPlugins = append(installedPlugins, pluginPkg)
	}
	return installedPlugins
}

// GetPlugins returns a combined list of all plugins (requested and local).
// Used by Run to determine which plugins to execute.
func GetPlugins() []*PluginPkg {
	output := make([]*PluginPkg, 0)
	localPlugins := getLocalPlugins()
	for _, plugin := range getRequestedPlugins() {
		if Contains(localPlugins, plugin.Name) {
			plugin.Installed = true
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

// getInstallablePlugins returns vetted plugins from the registry that are not already installed.
func getInstallablePlugins() ([]*PluginPkg, error) {
	remote, err := fetchVettedPlugins()
	if err != nil {
		return nil, err
	}
	local := getLocalPlugins()
	var out []*PluginPkg
	for _, vp := range remote {
		if vp.Name == "" || Contains(local, vp.Name) {
			continue
		}
		out = append(out, &PluginPkg{Name: vp.Name, Installable: true})
	}
	return out, nil
}

// getLocalAndRemotePlugins returns local plugins plus any vetted plugins from the registry that are not already installed.
func getLocalAndRemotePlugins() []*PluginPkg {
	plugins := getLocalPlugins()
	remote, _ := fetchVettedPlugins()
	for _, vp := range remote {
		if vp.Name == "" || Contains(plugins, vp.Name) {
			continue
		}
		plugins = append(plugins, &PluginPkg{Name: vp.Name, Installable: true})
	}
	return plugins
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
	return pluginNamesToVetted(resp.Plugins), nil
}

func pluginNamesToVetted(names []string) []*PluginPkg {
	list := make([]*PluginPkg, 0, len(names))
	for _, name := range names {
		if name != "" {
			list = append(list, &PluginPkg{Name: strings.TrimSpace(name)})
		}
	}
	return list
}

// Contains checks if a plugin with the given name exists in the slice.
func Contains(slice []*PluginPkg, search string) bool {
	for _, plugin := range slice {
		if plugin.Name == search {
			return true
		}
	}
	return false
}
