package command

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"
)

// PluginError retains an error object and the name of the pack that generated it.
type PluginError struct {
	Plugin string
	Err    error
}

// PluginErrors holds a list of errors and an Error() method
// so it adheres to the standard Error interface.
type PluginErrors struct {
	Errors []PluginError
}

func (e *PluginErrors) Error() string {
	return fmt.Sprintf("Service Pack Errors: %v", e.Errors)
}

// PluginPkg represents a plugin package with its metadata and execution state.
type PluginPkg struct {
	Name          string
	Path          string
	ServiceTarget string
	Command       *exec.Cmd
	Result        string

	Installable bool
	Installed   bool
	Requested   bool
	Successful  bool
	Error       error
}

func (p *PluginPkg) getBinary() (binaryName string, err error) {
	lookupName := filepath.Base(strings.ToLower(p.Name))
	if runtime.GOOS == "windows" && !strings.HasSuffix(lookupName, ".exe") {
		lookupName = fmt.Sprintf("%s.exe", lookupName)
	}
	plugins, _ := hcplugin.Discover(lookupName, viper.GetString("binaries-path"))
	if len(plugins) != 1 {
		err = fmt.Errorf("failed to locate requested plugin '%s' at path '%s'", lookupName, viper.GetString("binaries-path"))
		return
	}
	binaryName = plugins[0]
	return
}

func (p *PluginPkg) queueCmd() {
	cmd := exec.Command(p.Path)
	cmd.Args = append(cmd.Args,
		fmt.Sprintf("--config=%s", viper.GetString("config")),
		fmt.Sprintf("--loglevel=%s", viper.GetString("loglevel")),
		fmt.Sprintf("--service=%s", p.ServiceTarget),
	)
	p.Command = cmd
}

// closeClient logs the plugin result and kills the process.
func (p *PluginPkg) closeClient(serviceName string, client *hcplugin.Client, logger hclog.Logger) {
	if p.Successful {
		logger.Info(fmt.Sprintf("Plugin for %s completed successfully", serviceName))
	} else if p.Error != nil {
		logger.Error(fmt.Sprintf("Error from %s: %s", serviceName, p.Error))
	} else {
		logger.Warn(fmt.Sprintf("Unexpected exit from %s with no error or success", serviceName))
	}
	client.Kill()
}

// NewPluginPkg creates a new PluginPkg instance for the given plugin and service names.
func NewPluginPkg(pluginName string, serviceName string) *PluginPkg {
	plugin := &PluginPkg{
		Name: pluginName,
	}
	path, err := plugin.getBinary()
	if err != nil {
		plugin.Error = err
	}
	plugin.Path = path
	plugin.ServiceTarget = serviceName
	plugin.queueCmd()
	return plugin
}
