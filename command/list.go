package command

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// vettedPlugin holds a plugin name from the vetted list (used internally).
type vettedPlugin struct {
	Name string
}

// vettedListResponse is the shape of the vetted plugins JSON (object with message, updated, plugins).
type vettedListResponse struct {
	Message string   `json:"message"`
	Updated string   `json:"updated"`
	Plugins []string `json:"plugins"`
}

const defaultVettedPluginsURL = "https://revanite.io/privateer/vetted-plugins.json"

// getVettedPluginsURL returns the URL for the vetted plugins list. Use PVTR_REGISTRY_URL to override the default base (e.g. for a local or staging registry).
func getVettedPluginsURL() string {
	base := os.Getenv("PVTR_REGISTRY_URL")
	if base == "" {
		return defaultVettedPluginsURL
	}
	return strings.TrimSuffix(base, "/") + "/vetted-pvtr-plugins.json"
}

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
				plugins, err := getInstallablePlugins()
				if err != nil {
					_, _ = fmt.Fprintf(writer, "Error loading vetted plugins: %v\n", err)
					_ = writer.Flush()
					return
				}
				_, _ = fmt.Fprintln(writer, "Plugins that can be installed:")
				for _, pluginPkg := range plugins {
					_, _ = fmt.Fprintf(writer, "  - %s\n", pluginPkg.Name)
				}
			} else if viper.GetBool("installed") {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
				for _, pluginPkg := range getAllLocalPlugins() {
					_, _ = fmt.Fprintf(writer, "| %s \t | %t \t| %t \t|\n", pluginPkg.Name, pluginPkg.Available, pluginPkg.Requested)
				}
			} else if viper.GetBool("all") {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | Installed \t| In Current Config \t|")
				for _, pluginPkg := range getAllPlugins() {
					_, _ = fmt.Fprintf(writer, "| %s \t | %t \t| %t \t|\n", pluginPkg.Name, pluginPkg.Available, pluginPkg.Requested)
				}
			} else {
				_, _ = fmt.Fprintln(writer, "| Plugin \t | In Current Config \t|")
				for _, pluginPkg := range getRequestedPlugins() {
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
func GetInstalledPlugins() (installedPluginPackages []*PluginPkg) {
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
	installed := GetInstalledPlugins()
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

// getInstallablePlugins returns vetted plugins from the registry that are not already installed.
// On fetch error returns nil (no error surfaced).
func getInstallablePlugins() ([]*PluginPkg, error) {
	remote, err := fetchVettedPlugins()
	if err != nil {
		return nil, err
	}
	local := getAllLocalPlugins()
	var out []*PluginPkg
	for _, vp := range remote {
		if vp.Name == "" || Contains(local, vp.Name) {
			continue
		}
		out = append(out, &PluginPkg{Name: vp.Name, Available: false})
	}
	return out, nil
}

// getAllPlugins returns local plugins plus any vetted plugins from the registry that are not already installed.
func getAllPlugins() []*PluginPkg {
	plugins := getAllLocalPlugins()
	remote, _ := fetchVettedPlugins()
	for _, vp := range remote {
		if vp.Name == "" || Contains(plugins, vp.Name) {
			continue
		}
		plugins = append(plugins, &PluginPkg{Name: vp.Name, Available: false})
	}
	return plugins
}

// fetchVettedPlugins GETs the vetted plugins JSON. Returns (nil, error) on request or decode failure.
// Accepts object { "message", "updated", "plugins": ["name1", ...] } or top-level ["name1", ...].
func fetchVettedPlugins() ([]vettedPlugin, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(getVettedPluginsURL())
	if err != nil {
		return nil, fmt.Errorf("fetching vetted plugins: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vetted plugins endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	// Try object shape first (registry: message, updated, plugins).
	var out vettedListResponse
	if err := json.Unmarshal(body, &out); err == nil && len(out.Plugins) > 0 {
		return pluginNamesToVetted(out.Plugins), nil
	}
	// Try top-level array of strings.
	var arr []string
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		return pluginNamesToVetted(arr), nil
	}
	// Try generic object and look for plugins/Plugins key.
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}
	for _, key := range []string{"plugins", "Plugins"} {
		if v, ok := raw[key]; ok {
			if arr, ok := v.([]interface{}); ok {
				names := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok && s != "" {
						names = append(names, s)
					}
				}
				if len(names) > 0 {
					return pluginNamesToVetted(names), nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no \"plugins\" array found in response")
}

func pluginNamesToVetted(names []string) []vettedPlugin {
	list := make([]vettedPlugin, 0, len(names))
	for _, name := range names {
		if name != "" {
			list = append(list, vettedPlugin{Name: strings.TrimSpace(name)})
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
