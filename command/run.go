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

// Aliases for the canonical values in shared/ — kept here so command.TestPass
// etc. stay valid for existing callers.
const (
	TestPass      = shared.TestPass
	TestFail      = shared.TestFail
	Aborted       = shared.Aborted
	InternalError = shared.InternalError
	BadUsage      = shared.BadUsage
	NoTests       = shared.NoTests
)

// Across multi-plugin runs the most severe outcome wins.
var exitSeverity = map[int]int{
	TestPass:      0,
	TestFail:      1,
	BadUsage:      2,
	InternalError: 3,
}

func mergeExitCode(prev, next int) int {
	if exitSeverity[next] > exitSeverity[prev] {
		return next
	}
	return prev
}

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
				if !pluginPkg.Installed {
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
					pluginPkg.closeClient(serviceName, client, logger)
					return InternalError
				}
				// Request the plugin
				var rawPlugin interface{}
				rawPlugin, err = rpcClient.Dispense(shared.PluginName)
				if err != nil {
					logger.Error(fmt.Sprintf("internal error while dispensing RPC client: %s", err.Error()))
					pluginPkg.closeClient(serviceName, client, logger)
					return InternalError
				}
				// Execute plugin
				plugin := rawPlugin.(shared.Pluginer)
				logger.Trace(fmt.Sprintf("Starting Plugin %v: %s", runCount, pluginPkg.Name))
				pluginExitCode, response := plugin.Start()
				if response != nil {
					pluginPkg.Error = fmt.Errorf("plugin %s: %v", serviceName, response)
				}
				pluginPkg.Successful = pluginExitCode == TestPass
				if !pluginPkg.Successful {
					exitCode = mergeExitCode(exitCode, pluginExitCode)
				}
				pluginPkg.closeClient(serviceName, client, logger)
			}
		}
	}
	return exitCode
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
