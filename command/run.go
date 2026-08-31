package command

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/privateerproj/privateer-sdk/config"
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
//     A non-empty target scopes it to that service alone; other services are
//     ignored for both execution and validation, so a shared config with a
//     broken unrelated entry cannot block a targeted run.
//   - earlyExit is non-zero when Run must return immediately without executing:
//     NoTests when the plugin list is empty, BadUsage when the target names no
//     configured service or a requested plugin is not installed (errMsg is the
//     ready-to-log explanation).
//
// Pulling this decision out of the execution loop lets it be table-tested
// without go-plugin fakes. It validates the whole scoped list up front, so a
// config that requests an uninstalled plugin fails before any plugin runs.
func planRun(plugins []*PluginPkg, target string) (toRun []*PluginPkg, earlyExit int, errMsg string) {
	if len(plugins) == 0 {
		return nil, NoTests, ""
	}

	var requested []*PluginPkg
	for _, pluginPkg := range plugins {
		if pluginPkg.Requested {
			requested = append(requested, pluginPkg)
		}
	}

	if target != "" {
		var scoped []*PluginPkg
		for _, pluginPkg := range requested {
			// Viper lowercases config map keys (so ServiceTarget is lowercase),
			// while the target arrives with the user's casing; fold to match
			// the case-insensitive lookups everywhere else in the config layer.
			if strings.EqualFold(pluginPkg.ServiceTarget, target) {
				scoped = append(scoped, pluginPkg)
			}
		}
		if len(scoped) == 0 {
			return nil, BadUsage, unknownTargetMsg(target, requested)
		}
		requested = scoped
	}

	var missing []*PluginPkg
	for _, pluginPkg := range requested {
		if !pluginPkg.Installed {
			missing = append(missing, pluginPkg)
		}
	}
	if len(missing) > 0 {
		return nil, BadUsage, missingPluginsMsg(missing)
	}
	return requested, 0, ""
}

// unknownTargetMsg names the target that matched no configured service and
// lists the targets the config does define, so a typo is a one-glance fix.
func unknownTargetMsg(target string, requested []*PluginPkg) string {
	names := make([]string, 0, len(requested))
	for _, pluginPkg := range requested {
		names = append(names, pluginPkg.ServiceTarget)
	}
	if len(names) == 0 {
		return fmt.Sprintf("target %q is not defined in the config", target)
	}
	sort.Strings(names)
	return fmt.Sprintf("target %q is not defined in the config (available targets: %s)", target, strings.Join(names, ", "))
}

// missingPluginsMsg reports every requested-but-not-installed plugin with the
// targets that require it, sorted for deterministic output, so a shared config
// can be fixed in one pass instead of one failure per run.
func missingPluginsMsg(missing []*PluginPkg) string {
	byPlugin := make(map[string][]string, len(missing))
	for _, pluginPkg := range missing {
		byPlugin[pluginPkg.Name] = append(byPlugin[pluginPkg.Name], pluginPkg.ServiceTarget)
	}
	names := make([]string, 0, len(byPlugin))
	for name := range byPlugin {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		targets := byPlugin[name]
		sort.Strings(targets)
		// A service entry with no plugin: key reaches here with an empty name;
		// say so rather than render a blank.
		if name == "" {
			name = "no plugin configured"
		}
		parts = append(parts, fmt.Sprintf("%s (required by targets: %s)", name, strings.Join(targets, ", ")))
	}
	noun := "plugin that is"
	if len(names) > 1 {
		noun = "plugins that are"
	}
	return fmt.Sprintf("requested %s not installed: %s", noun, strings.Join(parts, ", "))
}

// Run executes all plugins with handling for the command line.
//
// Deprecated: use harness.Run instead. This will be removed once the pvtr CLI
// migrates to the command/harness import path.
func Run(logger hclog.Logger, getPlugins func() []*PluginPkg) (exitCode int) {
	logger.Trace(fmt.Sprintf(
		"Using bin: %s", viper.GetString("binaries-path")))

	toRun, earlyExit, errMsg := planRun(getPlugins(), config.TargetName())
	switch earlyExit {
	case NoTests:
		logger.Error(fmt.Sprintf("no plugins were requested in config: %s", viper.GetString("binaries-path")))
		return NoTests
	case BadUsage:
		logger.Error(errMsg)
		return BadUsage
	}

	// runParallel and runSequential are mock tested with runFunc
	// as a param, so we need to wrap runOne here to use it for real.
	run := func(num int, pluginPkg *PluginPkg) (int, bool) {
		return runOne(logger, num, pluginPkg)
	}
	limit, err := concurrencyLimit()
	if err != nil {
		logger.Error(err.Error())
		return BadUsage
	}
	if limit != 1 {
		return runParallel(toRun, limit, run)
	}
	return runSequential(toRun, run)
}

// concurrencyLimit returns the requested plugin pool size, or an error for
// anything that is not a non-negative integer.
func concurrencyLimit() (int, error) {
	viper.SetDefault("concurrency", 1)
	raw := viper.GetString("concurrency")
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return 0, fmt.Errorf("concurrency must be a non-negative integer, got %q", raw)
	}
	return limit, nil
}

// runSequential executes plugins one at a time in input order, merging exit
// codes by severity. It returns immediately only on a host-level failure.
func runSequential(toRun []*PluginPkg, run runFunc) (exitCode int) {
	for i, pluginPkg := range toRun {
		code, fatal := run(i+1, pluginPkg)
		if fatal {
			return InternalError
		}
		exitCode = mergeExitCode(exitCode, code)
	}
	return exitCode
}

// maxConcurrency is the safety cap on an explicit limit: 100, or the CPU
// count when the host is larger than that. It exists so a goofy config value
// can't fork-bomb the machine, while a host big enough to handle the request
// is allowed to.
const maxConcurrency = 100

// runParallel executes plugins concurrently, at most limit at a time. A limit
// of 0 or less means one per CPU. Unlike the sequential path, it never aborts early.
func runParallel(toRun []*PluginPkg, limit int, run runFunc) (exitCode int) {
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	limit = min(limit, max(maxConcurrency, runtime.NumCPU()))
	codes := make([]int, len(toRun))
	var g errgroup.Group
	g.SetLimit(limit)
	for i, pluginPkg := range toRun {
		g.Go(func() error {
			codes[i], _ = run(i+1, pluginPkg)
			return nil
		})
	}
	_ = g.Wait() // run never returns an error; Wait is only the barrier
	for _, code := range codes {
		exitCode = mergeExitCode(exitCode, code)
	}
	return exitCode
}

// runFunc executes one plugin end to end. runOne is the only production
// implementation; this is a parameter so the sequencing and pooling can
// be mock tested without spawning plugin subprocesses.
type runFunc func(num int, pluginPkg *PluginPkg) (int, bool)

// runOne executes a single plugin subprocess end to end recording the outcome on pluginPkg.
// It returns the exit code and whether the failure was host-level.
// It only touches pluginPkg's own fields, so distinct plugins are safe to run concurrently.
func runOne(logger hclog.Logger, num int, pluginPkg *PluginPkg) (code int, fatal bool) {
	serviceName := pluginPkg.ServiceTarget
	client := newClient(pluginPkg.Command, logger)

	rpcClient, err := client.Client()
	if err != nil {
		logger.Error(fmt.Sprintf("internal error while initializing %s RPC client: %s", serviceName, err))
		pluginPkg.closeClient(serviceName, client, logger)
		return InternalError, true
	}
	rawPlugin, err := rpcClient.Dispense(shared.PluginName)
	if err != nil {
		logger.Error(fmt.Sprintf("internal error while dispensing RPC client: %s", err.Error()))
		pluginPkg.closeClient(serviceName, client, logger)
		return InternalError, true
	}
	plugin := rawPlugin.(shared.Pluginer)
	logger.Trace(fmt.Sprintf("Starting Plugin %v: %s", num, pluginPkg.Name))

	pluginExitCode, response := plugin.Start()
	if response != nil {
		pluginPkg.Error = fmt.Errorf("plugin %s: %v", serviceName, response)
	}
	pluginPkg.Successful = pluginExitCode == TestPass
	pluginPkg.closeClient(serviceName, client, logger)
	return pluginExitCode, false
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
