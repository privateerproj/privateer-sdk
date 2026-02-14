package command

import (
	"fmt"
	"os"
	"os/exec"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/shared"
)

// Exit codes for plugin execution results.
const (
	TestPass      = iota // TestPass indicates all tests passed.
	TestFail             // TestFail indicates one or more tests failed.
	Aborted              // Aborted indicates execution was aborted.
	InternalError        // InternalError indicates an internal error occurred.
	BadUsage             // BadUsage indicates incorrect command usage.
	NoTests              // NoTests indicates no tests were found to run.
)

// Run executes all plugins with handling for the command line.
func Run(logger hclog.Logger, getPlugins func() []*PluginPkg) (exitCode int) {
	logger.Trace(fmt.Sprintf(
		"Using bin: %s", viper.GetString("binaries-path")))

	plugins := getPlugins()
	if len(plugins) == 0 {
		logger.Error(fmt.Sprintf("no plugins were requested in config: %s", viper.GetString("binaries-path")))
		return NoTests
	}

	// Run all plugins
	var runCount int
	for serviceName := range viper.GetStringMap("services") {
		servicePluginName := viper.GetString(fmt.Sprintf("services.%s.plugin", serviceName))
		for _, pluginPkg := range plugins {
			if pluginPkg.Name == servicePluginName {
				if !pluginPkg.Available {
					logger.Error(fmt.Sprintf("requested plugin that is not installed: %s", pluginPkg.Name))
					return BadUsage
				}
				runCount++
				client := newClient(pluginPkg.Command, logger)
				// Connect via RPC
				var rpcClient hcplugin.ClientProtocol
				rpcClient, err := client.Client()
				if err != nil {
					logger.Error(fmt.Sprintf("internal error while initializing %s RPC client: %s", serviceName, err))
					closeClient(pluginPkg, serviceName, client, logger)
					return InternalError
				}
				// Request the plugin
				var rawPlugin interface{}
				rawPlugin, err = rpcClient.Dispense(shared.PluginName)
				if err != nil {
					logger.Error(fmt.Sprintf("internal error while dispensing RPC client: %s", err.Error()))
					closeClient(pluginPkg, serviceName, client, logger)
					return InternalError
				}
				// Execute plugin
				plugin := rawPlugin.(shared.Pluginer)
				logger.Trace(fmt.Sprintf("Starting Plugin %v: %s", runCount, pluginPkg.Name))
				response := plugin.Start()
				if response != nil {
					pluginPkg.Error = fmt.Errorf("tests failed in plugin %s: %v", serviceName, response)
					exitCode = TestFail
				} else {
					pluginPkg.Successful = true
				}
				closeClient(pluginPkg, serviceName, client, logger)
			}
		}
	}
	return exitCode
}

func closeClient(pluginPkg *PluginPkg, serviceName string, client *hcplugin.Client, logger hclog.Logger) {
	// Close the client: this doesn't work via defer because it leaves the plugin running while the next begins
	if pluginPkg.Successful {
		logger.Info(fmt.Sprintf("Plugin for %s completed successfully", serviceName))
	} else if pluginPkg.Error != nil {
		logger.Error(pluginPkg.Error.Error())
	} else {
		logger.Error(fmt.Sprintf("unexpected exit while attempting to run package: %v", pluginPkg))
	}
	client.Kill()
}

// newClient handles the lifecycle of a plugin application.
// Plugin hosts should use one Client for each plugin executable
// (this is different from the client that manages gRPC).
func newClient(cmd *exec.Cmd, logger hclog.Logger) *hcplugin.Client {
	var pluginMap = map[string]hcplugin.Plugin{
		shared.PluginName: &shared.Plugin{},
	}
	var handshakeConfig = shared.GetHandshakeConfig()
	return hcplugin.NewClient(&hcplugin.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		Cmd:             cmd,
		Logger:          logger,
		SyncStdout:      os.Stdout,
		SyncStderr:      os.Stderr,
	})
}

