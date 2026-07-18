package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/shared"
)

// benchmarkCmd returns the `pvtr benchmark` command. It takes the plugin binary
// path directly (not a manifest coordinate, so a freshly-built plugin needs no
// install), runs it once in benchmark mode, and renders the report the plugin
// writes. An older SDK without instrumentation produces no report.
func benchmarkCmd(writerFn func() Writer) *cobra.Command {
	var (
		service     string
		jsonOut     bool
		writeDir    string
		payloadOnly bool
	)

	benchmarkCmd := &cobra.Command{
		Use:   "benchmark <plugin-binary>",
		Short: "Time a plugin's payload retrieval and every step, and print a cost breakdown.",
		Long: "Run a plugin once with benchmark instrumentation enabled and print a breakdown " +
			"of where the time went: payload retrieval (the loader) separately from each " +
			"individual assessment step, using monotonic sub-millisecond durations.\n\n" +
			"Takes the path to a compiled plugin binary — the plugin runs as a separate " +
			"process, so build it first (`make build`, or `go build -o <name> .`) and pass " +
			"the resulting executable. The named --service must exist in the config file so " +
			"the plugin has its vars, catalogs, and applicability.\n\n" +
			"Use --payload-only to stop after payload retrieval and time just the loader — " +
			"assessment steps consume the payload, so they cannot run without it and are " +
			"skipped. Use --json for a machine-readable report suitable for diffing plugin " +
			"versions in CI.",
		Args: cobra.ExactArgs(1),
		// runtime failures shouldn't reprint usage text
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if service == "" {
				return fmt.Errorf("--service is required: the plugin needs a configured service to evaluate")
			}
			binPath, err := resolvePluginBinary(args[0])
			if err != nil {
				return err
			}
			if writeDir == "" {
				writeDir, err = os.MkdirTemp("", "pvtr-benchmark-")
				if err != nil {
					return fmt.Errorf("creating benchmark output directory: %w", err)
				}
			}

			report, exitCode, err := runBenchmark(binPath, service, writeDir, payloadOnly)
			if err != nil {
				return err
			}

			if jsonOut {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshaling benchmark report: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				w := writerFn()
				renderBenchmark(w, report, exitCode, writeDir)
				_ = w.Flush()
			}
			return nil
		},
	}
	benchmarkCmd.Flags().StringVarP(&service, "service", "s", "", "Named service from the config to evaluate (required)")
	benchmarkCmd.Flags().BoolVar(&jsonOut, "json", false, "Emit the machine-readable benchmark report as JSON")
	benchmarkCmd.Flags().StringVarP(&writeDir, "write-directory", "w", "", "Directory for the run's results and report (default: a temp directory)")
	benchmarkCmd.Flags().BoolVar(&payloadOnly, "payload-only", false, "Stop after payload retrieval and time only the loader (skip assessment steps)")
	return benchmarkCmd
}

// resolvePluginBinary validates the path and returns it absolute, so exec never
// does a $PATH lookup for a bare name.
func resolvePluginBinary(arg string) (string, error) {
	binPath, err := filepath.Abs(arg)
	if err != nil {
		return "", fmt.Errorf("resolving plugin binary path %q: %w", arg, err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		return "", fmt.Errorf("plugin binary %q: %w", arg, err)
	}
	if info.IsDir() {
		// most likely mistake: pointing at the source tree instead of the binary
		if isPluginSource(binPath) {
			return "", fmt.Errorf(
				"%q is a plugin source directory, not a compiled binary — build it first (`make build`, or `go build -o <name> .`) and pass the resulting executable", arg)
		}
		return "", fmt.Errorf("plugin binary %q is a directory, not an executable", arg)
	}
	// Windows has no exec bit; let the exec attempt be the arbiter there.
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("plugin binary %q is not executable", arg)
	}
	return binPath, nil
}

// isPluginSource reports whether dir looks like a Go plugin source tree, so the
// error can point at the build step.
func isPluginSource(dir string) bool {
	for _, marker := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// runBenchmark executes the plugin once in benchmark mode and returns the
// report it wrote. Failed controls are not errors — benchmarking a failing run
// is valid — so the plugin's exit code is returned alongside the report.
func runBenchmark(binPath, service, writeDir string, payloadOnly bool) (*pluginkit.BenchmarkReport, int, error) {
	pluginCmd := exec.Command(binPath,
		fmt.Sprintf("--config=%s", viper.GetString("config")),
		fmt.Sprintf("--loglevel=%s", viper.GetString("loglevel")),
		fmt.Sprintf("--service=%s", service),
		fmt.Sprintf("--write-directory=%s", writeDir),
	)
	// plugins have no --benchmark flag; instrumentation travels as env
	pluginCmd.Env = append(os.Environ(), "PVTR_BENCHMARK=true")
	if payloadOnly {
		pluginCmd.Env = append(pluginCmd.Env, "PVTR_BENCHMARK_PAYLOAD_ONLY=true")
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "benchmark",
		Level:  hclog.LevelFromString(viper.GetString("loglevel")),
		Output: os.Stderr,
	})
	client := hcplugin.NewClient(&hcplugin.ClientConfig{
		HandshakeConfig: shared.GetHandshakeConfig(),
		Plugins:         map[string]hcplugin.Plugin{shared.PluginName: &shared.Plugin{}},
		Cmd:             pluginCmd,
		Logger:          logger,
		SyncStdout:      os.Stdout,
		SyncStderr:      os.Stderr,
	})
	defer client.Kill()

	// brackets startup, handshake, and RPC — what the plugin-side total can't see
	wallStart := time.Now()
	rpcClient, err := client.Client()
	if err != nil {
		return nil, InternalError, fmt.Errorf("initializing plugin RPC client: %w", err)
	}
	rawPlugin, err := rpcClient.Dispense(shared.PluginName)
	if err != nil {
		return nil, InternalError, fmt.Errorf("dispensing plugin RPC client: %w", err)
	}
	plugin := rawPlugin.(shared.Pluginer)
	exitCode, startErr := plugin.Start()
	wallClock := time.Since(wallStart)

	reportPath := filepath.Join(writeDir, service, pluginkit.BenchmarkFileName)
	data, readErr := os.ReadFile(reportPath)
	report, err := parseBenchmarkReport(data, readErr, startErr, reportPath)
	if err != nil {
		return nil, exitCode, err
	}
	report.WallClockNs = wallClock.Nanoseconds()
	return report, exitCode, nil
}

// parseBenchmarkReport returns the report or the best explanation of its
// absence. Split from runBenchmark so the diagnosis is unit-testable without
// spawning a plugin. readErr is from reading the file, startErr from the
// plugin's Start; a missing report means a plugin failure or an SDK too old to
// instrument.
func parseBenchmarkReport(data []byte, readErr, startErr error, reportPath string) (*pluginkit.BenchmarkReport, error) {
	if readErr != nil {
		if startErr != nil {
			return nil, fmt.Errorf("plugin run failed before producing a benchmark report: %w", startErr)
		}
		return nil, fmt.Errorf(
			"the run completed but no benchmark report was found at %s — the plugin was likely built against a privateer-sdk without benchmark instrumentation; rebuild it with a newer SDK", reportPath)
	}
	report := &pluginkit.BenchmarkReport{}
	if err := json.Unmarshal(data, report); err != nil {
		return nil, fmt.Errorf("parsing benchmark report %s: %w", reportPath, err)
	}
	return report, nil
}

type benchmarkRow struct {
	label      string
	detail     string
	durationNs int64
}

// renderBenchmark prints the breakdown: every segment sorted by cost, with a
// share-of-total column against the plugin-side total.
func renderBenchmark(w Writer, report *pluginkit.BenchmarkReport, exitCode int, writeDir string) {
	version := report.PluginVersion
	if version == "" {
		version = "unversioned"
	}
	_, _ = fmt.Fprintf(w, "Benchmark: %s (%s) service=%s\n", report.PluginName, version, report.ServiceName)
	_, _ = fmt.Fprintf(w, "Wall clock %v; plugin total %v; exit code %d; results in %s\n",
		formatNs(report.WallClockNs), formatNs(report.TotalDurationNs), exitCode, writeDir)
	if report.APICalls != nil {
		_, _ = fmt.Fprintf(w, "API calls reported by payload: %d\n", *report.APICalls)
	}
	_, _ = fmt.Fprintln(w)

	var rows []benchmarkRow
	var attributedNs int64
	for _, l := range report.Loaders {
		rows = append(rows, benchmarkRow{
			label:      "payload retrieval (" + l.Scope + ")",
			detail:     shortFuncName(l.Func),
			durationNs: l.DurationNs,
		})
		attributedNs += l.DurationNs
	}
	multiSuite := len(report.Suites) > 1
	for _, s := range report.Suites {
		// suite duration minus its steps is evaluation-loop overhead; give it its
		// own row so it isn't mislabeled as unattributed startup/writing cost
		var suiteStepsNs int64
		for _, st := range s.Steps {
			suiteStepsNs += st.DurationNs
		}
		if overhead := s.DurationNs - suiteStepsNs; overhead > 0 {
			rows = append(rows, benchmarkRow{
				label:      "evaluation overhead (" + s.CatalogId + ")",
				detail:     "applicability, aggregation, logging",
				durationNs: overhead,
			})
			attributedNs += overhead
		}
		for _, st := range s.Steps {
			label := st.RequirementId
			if multiSuite {
				label = s.CatalogId + " " + label
			}
			if st.StepIndex > 0 {
				label = fmt.Sprintf("%s step[%d]", label, st.StepIndex)
			}
			rows = append(rows, benchmarkRow{
				label:      label,
				detail:     shortFuncName(st.Step),
				durationNs: st.DurationNs,
			})
			attributedNs += st.DurationNs
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].durationNs > rows[j].durationNs })
	if unattributed := report.TotalDurationNs - attributedNs; unattributed > 0 {
		rows = append(rows, benchmarkRow{
			label:      "unattributed",
			detail:     "config, catalog parsing, result writing",
			durationNs: unattributed,
		})
	}

	_, _ = fmt.Fprintln(w, "SEGMENT\tFUNCTION\tDURATION\tSHARE")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", row.label, row.detail, formatNs(row.durationNs), share(row.durationNs, report.TotalDurationNs))
	}
}

// formatNs renders a duration human-readably, rounded but sub-ms accurate.
func formatNs(ns int64) time.Duration {
	d := time.Duration(ns)
	switch {
	case d >= time.Second:
		return d.Round(10 * time.Millisecond)
	case d >= time.Millisecond:
		return d.Round(100 * time.Microsecond)
	case d >= time.Microsecond:
		return d.Round(time.Microsecond)
	default:
		return d // sub-µs: exact ns, never a misleading 0
	}
}

func share(part, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", float64(part)/float64(total)*100)
}

// shortFuncName trims a function symbol to its last path segment (package.Func).
func shortFuncName(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
