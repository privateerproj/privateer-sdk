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

// planRun decides, without executing anything, what Run should do with the
// resolved plugin list:
//   - toRun is the requested-and-installed plugins to execute, in input order.
//   - earlyExit is non-zero when Run must return immediately without executing:
//     NoTests when the plugin list is empty, BadUsage when a requested plugin
//     is not installed (culprit names it, for the log line).
//
// Pulling this decision out of the execution loop lets it be table-tested
// without go-plugin fakes. It validates the whole list up front, so a config
// that requests an uninstalled plugin now fails before any plugin runs.
// Previously a requested+installed plugin earlier in the (map-ordered, so
// non-deterministic) slice could execute before the BadUsage abort.
func planRun(plugins []*PluginPkg) (toRun []*PluginPkg, earlyExit int, culprit *PluginPkg) {
	if len(plugins) == 0 {
		return nil, NoTests, nil
	}
	for _, pluginPkg := range plugins {
		if !pluginPkg.Requested {
			continue
		}
		if !pluginPkg.Installed {
			return nil, BadUsage, pluginPkg
		}
		toRun = append(toRun, pluginPkg)
	}
	return toRun, 0, nil
}

// Run executes all plugins with handling for the command line.
//
// Deprecated: use harness.Run instead. This will be removed once the pvtr CLI
// migrates to the command/harness import path.
func Run(logger hclog.Logger, getPlugins func() []*PluginPkg) (exitCode int) {
	logger.Trace(fmt.Sprintf(
		"Using bin: %s", viper.GetString("binaries-path")))

	toRun, earlyExit, culprit := planRun(getPlugins())
	switch earlyExit {
	case NoTests:
		logger.Error(fmt.Sprintf("no plugins were requested in config: %s", viper.GetString("binaries-path")))
		return NoTests
	case BadUsage:
		logger.Error(fmt.Sprintf("requested plugin that is not installed: %s", culprit.Name))
		return BadUsage
	}

	var runCount int
	for _, pluginPkg := range toRun {
		serviceName := pluginPkg.ServiceTarget
		runCount++
		client := newClient(pluginPkg.Command, logger)
		var rpcClient hcplugin.ClientProtocol
		rpcClient, err := client.Client()
		if err != nil {
			logger.Error(fmt.Sprintf("internal error while initializing %s RPC client: %s", serviceName, err))
			pluginPkg.closeClient(serviceName, client, logger)
			return InternalError
		}
		var rawPlugin interface{}
		rawPlugin, err = rpcClient.Dispense(shared.PluginName)
		if err != nil {
			logger.Error(fmt.Sprintf("internal error while dispensing RPC client: %s", err.Error()))
			pluginPkg.closeClient(serviceName, client, logger)
			return InternalError
		}
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
