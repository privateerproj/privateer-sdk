package command

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const vettedPluginsURL = "https://revanite.io/privateer/vetted-pvtr-plugins.json"

// Writer is an interface for output operations that supports writing and flushing.
type Writer interface {
	io.Writer
	Flush() error
}

// GetListCmd returns the list command that can be added to a root command
func GetListCmd(writer Writer) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Consult the Charts! List all plugins that have been installed.",
		Run: func(cmd *cobra.Command, args []string) {
			if viper.GetBool("installable") {
				_, _ = fmt.Fprintln(writer, "| Plugin \t|")
				for _, pluginPkg := range GetPlugins() {
					_, _ = fmt.Fprintf(writer, "| %s \t|\n", pluginPkg.Name)
				}
			} else if viper.GetBool("installed") {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
				for _, pluginPkg := range GetPlugins() {
					_, _ = fmt.Fprintf(writer, "| %s \t | %t \t| %t \t|\n", pluginPkg.Name, pluginPkg.Available, pluginPkg.Requested)
				}
			} else if viper.GetBool("all") {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
				for _, pluginPkg := range GetPlugins() {
					_, _ = fmt.Fprintf(writer, "| %s \t | %t \t| %t \t|\n", pluginPkg.Name, pluginPkg.Available, pluginPkg.Requested)
				}
			} else {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | In Current Config \t|")
				for _, pluginPkg := range GetPlugins() {
					if pluginPkg.Available {
						_, _ = fmt.Fprintf(writer, "| %s \t | %t \t|\n", pluginPkg.Name, pluginPkg.Requested)
					}
				}
			}
			err := writer.Flush()
			if err != nil {
				log.Printf("Error flushing writer: %v", err)
			}
		},
	}
	return listCmd
}

func SetListCmdFlags(listCmd *cobra.Command) {
	listCmd.PersistentFlags().BoolP("all", "a", false, "Review the Fleet! List all plugins that have been installed or requested in the current config")
	listCmd.PersistentFlags().Bool("installable", false, "List vetted plugins that are not yet installed")
	listCmd.PersistentFlags().Bool("installed", false, "List installed and requested plugins (local only)")
	_ = viper.BindPFlag("all", listCmd.PersistentFlags().Lookup("all"))
	_ = viper.BindPFlag("installable", listCmd.PersistentFlags().Lookup("installable"))
	_ = viper.BindPFlag("installed", listCmd.PersistentFlags().Lookup("installed"))
}

// GetPlugins returns the plugin list appropriate for the current flags (all, installable, installed).
func GetPlugins() []*PluginPkg {
	if viper.GetBool("all") {
		return getAllPlugins()
	}
	if viper.GetBool("installable") {
		return getInstallablePlugins()
	}
	if viper.GetBool("installed") {
		return getAllLocalPlugins()
	}
	return getRequestedPlugins()
}

// getRequestedPlugins returns a list of plugin names requested in the config.
func getRequestedPlugins() (requestedPluginPackages []*PluginPkg) {
	services := viper.GetStringMap("services")
	for serviceName := range services {
		pluginName := viper.GetString("services." + serviceName + ".plugin")
		pluginPkg := NewPluginPkg(pluginName, serviceName)
		pluginPkg.Requested = true
		requestedPluginPackages = append(requestedPluginPackages, pluginPkg)
	}
	return requestedPluginPackages
}

// getInstalledPlugins returns a list of plugins found in the binaries path.
func getInstalledPlugins() (installedPluginPackages []*PluginPkg) {
	pluginPaths, _ := hcplugin.Discover("*", viper.GetString("binaries-path"))
	for _, pluginPath := range pluginPaths {
		pluginPkg := NewPluginPkg(path.Base(pluginPath), "")
		pluginPkg.Available = true
		if strings.Contains(pluginPkg.Name, "privateer") {
			continue
		}
		installedPluginPackages = append(installedPluginPackages, pluginPkg)
	}
	return installedPluginPackages
}

// getAllLocalPlugins returns a combined list of all plugins (requested and installed).
func getAllLocalPlugins() []*PluginPkg {
	output := make([]*PluginPkg, 0)
	installed := getInstalledPlugins()
	for _, plugin := range getRequestedPlugins() {
		if Contains(installed, plugin.Name) {
			plugin.Available = true
		}
		output = append(output, plugin)
	}
	for _, plugin := range installed {
		if !Contains(output, plugin.Name) {
			output = append(output, plugin)
		}
	}
	return output
}

// vettedPlugin is the shape of an entry in the vetted plugins JSON.
type vettedPlugin struct {
	Name string `json:"name"`
}

// getInstallablePlugins returns vetted plugins from the registry that are not already installed.
func getInstallablePlugins() []*PluginPkg {
	local := getAllLocalPlugins()
	var out []*PluginPkg
	for _, vp := range fetchVettedPlugins() {
		if vp.Name == "" || Contains(local, vp.Name) {
			continue
		}
		out = append(out, &PluginPkg{Name: vp.Name, Available: false})
	}
	return out
}

// getAllPlugins returns local plugins plus any vetted plugins from the registry that are not already installed.
func getAllPlugins() []*PluginPkg {
	plugins := getAllLocalPlugins()
	remote := fetchVettedPlugins()
	for _, vp := range remote {
		if vp.Name == "" || Contains(plugins, vp.Name) {
			continue
		}
		plugins = append(plugins, &PluginPkg{Name: vp.Name, Available: false})
	}
	return plugins
}

// fetchVettedPlugins GETs the vetted plugins JSON and returns the list (empty on error).
func fetchVettedPlugins() []vettedPlugin {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(vettedPluginsURL)
	if err != nil {
		log.Printf("Failed to fetch vetted plugins: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Vetted plugins endpoint returned %d", resp.StatusCode)
		return nil
	}
	var list []vettedPlugin
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		log.Printf("Failed to decode vetted plugins JSON: %v", err)
		return nil
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
