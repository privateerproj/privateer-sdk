package command

import (
	"fmt"
	"os/exec"
	"path/filepath"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
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
	Version       string // requested version; "" means "latest installed"
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

// getBinary resolves the on-disk path of the plugin binary from the manifest.
// When Version is set it requires that exact version; otherwise it selects the
// latest installed version. This replaces filesystem-glob discovery so that
// multiple installed versions (each at its own coordinate/version/entrypoint
// path) resolve unambiguously by name+version.
func (p *PluginPkg) getBinary() (binaryPath string, err error) {
	binariesPath := viper.GetString("binaries-path")
	m, err := manifest.Load(binariesPath)
	if err != nil {
		return "", fmt.Errorf("loading plugin manifest: %w", err)
	}

	var entry *manifest.Plugin
	if p.Version != "" {
		if entry = m.FindVersion(p.Name, p.Version); entry == nil {
			return "", fmt.Errorf("plugin %s@%s is not installed in %s", p.Name, p.Version, binariesPath)
		}
	} else if entry = m.Latest(p.Name); entry == nil {
		return "", fmt.Errorf("plugin %s is not installed in %s", p.Name, binariesPath)
	}
	return filepath.Join(binariesPath, entry.BinaryPath), nil
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
func (p *PluginPkg) closeClient(serviceName string, client pluginClient, logger hclog.Logger) {
	if p.Successful {
		logger.Info(fmt.Sprintf("Plugin for %s completed successfully", serviceName))
	} else if p.Error != nil {
		logger.Error(fmt.Sprintf("Error from %s: %s", serviceName, p.Error))
	} else {
		logger.Warn(fmt.Sprintf("Unexpected exit from %s with no error or success", serviceName))
	}
	client.Kill()
}

// NewPluginPkg creates a new PluginPkg for the given plugin name, requested
// version (empty for "latest installed"), and service. It resolves the binary
// from the manifest: on success the package is marked Installed with its Path
// set; on failure Error is recorded and Installed stays false.
func NewPluginPkg(pluginName, version, serviceName string) *PluginPkg {
	plugin := &PluginPkg{
		Name:          pluginName,
		Version:       version,
		ServiceTarget: serviceName,
	}
	path, err := plugin.getBinary()
	if err != nil {
		plugin.Error = err
	} else {
		plugin.Path = path
		plugin.Installed = true
		plugin.queueCmd()
	}
	return plugin
}
