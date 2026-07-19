package harness

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/pluginkit"
)

// benchBufWriter is a minimal command.Writer capturing output for assertions.
type benchBufWriter struct{ bytes.Buffer }

func (b *benchBufWriter) Flush() error { return nil }

func TestGetBenchmarkCmd_Shape(t *testing.T) {
	cmd := GetBenchmarkCmd(func() Writer { return &benchBufWriter{} })
	if cmd.Use != "benchmark <plugin-binary>" {
		t.Errorf("unexpected Use: %q", cmd.Use)
	}
	for _, flag := range []string{"service", "json", "write-directory", "payload-only"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag", flag)
		}
	}
}

func TestBenchmarkCmd_RequiresService(t *testing.T) {
	cmd := GetBenchmarkCmd(func() Writer { return &benchBufWriter{} })
	cmd.SetArgs([]string{"/does/not/matter"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--service is required") {
		t.Errorf("expected missing-service error, got: %v", err)
	}
}

func TestResolvePluginBinary(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		if _, err := resolvePluginBinary(filepath.Join(tmpDir, "nope")); err == nil {
			t.Error("expected error for missing binary")
		}
	})

	t.Run("directory", func(t *testing.T) {
		if _, err := resolvePluginBinary(tmpDir); err == nil {
			t.Error("expected error for directory")
		}
	})

	t.Run("plugin source directory points at the build step", func(t *testing.T) {
		srcDir := filepath.Join(tmpDir, "src")
		if err := os.Mkdir(srcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := resolvePluginBinary(srcDir)
		if err == nil || !strings.Contains(err.Error(), "source directory") {
			t.Errorf("expected a source-directory diagnosis, got: %v", err)
		}
	})

	t.Run("not executable", func(t *testing.T) {
		p := filepath.Join(tmpDir, "plain")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := resolvePluginBinary(p); err == nil {
			t.Error("expected error for non-executable file")
		}
	})

	t.Run("executable resolves to absolute", func(t *testing.T) {
		p := filepath.Join(tmpDir, "bin")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		resolved, err := resolvePluginBinary(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !filepath.IsAbs(resolved) {
			t.Errorf("expected absolute path, got %q", resolved)
		}
	})
}

func TestParseBenchmarkReport(t *testing.T) {
	readErr := os.ErrNotExist
	startErr := errors.New("loader exploded")

	t.Run("plugin failure explains a missing report", func(t *testing.T) {
		_, err := parseBenchmarkReport(nil, readErr, startErr, "/x/benchmark.json")
		if err == nil || !strings.Contains(err.Error(), "failed before producing a benchmark report") {
			t.Errorf("expected the plugin failure to be surfaced, got: %v", err)
		}
		if !errors.Is(err, startErr) {
			t.Errorf("expected the plugin's error to be wrapped, got: %v", err)
		}
	})

	t.Run("missing report after a clean run points at an old SDK", func(t *testing.T) {
		_, err := parseBenchmarkReport(nil, readErr, nil, "/x/benchmark.json")
		// assert on the remedy, not just the diagnosis — the actionable half is
		// the part users need and the part easiest to lose in a reword
		if err == nil || !strings.Contains(err.Error(), "benchmark instrumentation") ||
			!strings.Contains(err.Error(), "rebuilding it against a newer privateer-sdk") {
			t.Errorf("expected the old-SDK diagnosis and its remedy, got: %v", err)
		}
	})

	t.Run("malformed report", func(t *testing.T) {
		_, err := parseBenchmarkReport([]byte("{not json"), nil, nil, "/x/benchmark.json")
		if err == nil || !strings.Contains(err.Error(), "parsing benchmark report") {
			t.Errorf("expected a parse error, got: %v", err)
		}
	})

	t.Run("valid report", func(t *testing.T) {
		report, err := parseBenchmarkReport([]byte(`{"plugin-name":"p","total-duration-ns":5}`), nil, nil, "/x/benchmark.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.PluginName != "p" || report.TotalDurationNs != 5 {
			t.Errorf("report not parsed: %+v", report)
		}
	})
}

func TestRenderBenchmark(t *testing.T) {
	calls := 22
	report := &pluginkit.BenchmarkReport{
		Schema:        pluginkit.BenchmarkSchema,
		PluginName:    "github-repo",
		PluginVersion: "v1.0.0",
		ServiceName:   "myrepo",
		Loaders: []pluginkit.LoaderTiming{
			{Scope: "orchestrator", Func: "github.com/ossf/pvtr-github-repo/data.Loader", DurationNs: 9_000_000_000},
		},
		Suites: []pluginkit.SuiteTiming{
			{
				CatalogId:  "osps-baseline",
				DurationNs: 1_500_000_000,
				Steps: []pluginkit.StepTiming{
					{ControlId: "OSPS-AC-01", RequirementId: "OSPS-AC-01.01", StepIndex: 0, Step: "github.com/ossf/pvtr-github-repo/eval.SlowStep", Result: "Passed", DurationNs: 1_200_000_000},
					{ControlId: "OSPS-AC-02", RequirementId: "OSPS-AC-02.01", StepIndex: 0, Step: "github.com/ossf/pvtr-github-repo/eval.FastStep", Result: "Failed", DurationNs: 5_000_000},
				},
			},
		},
		TotalDurationNs: 11_000_000_000,
		APICalls:        &calls,
		WallClockNs:     11_400_000_000,
	}

	w := &benchBufWriter{}
	renderBenchmark(w, report, 0, "/tmp/out")
	out := w.String()

	for _, want := range []string{
		"github-repo (v1.0.0) service=myrepo",
		"payload retrieval (orchestrator)",
		"data.Loader",
		"OSPS-AC-01.01",
		"eval.SlowStep",
		"unattributed",
		"API calls reported by payload: 22",
		"81.8%", // loader share of 11s total
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\n---\n%s", want, out)
		}
	}

	// Sorted by cost: the loader line must come before the slow step, which
	// must come before the fast step.
	loaderIdx := strings.Index(out, "payload retrieval")
	slowIdx := strings.Index(out, "eval.SlowStep")
	fastIdx := strings.Index(out, "eval.FastStep")
	if loaderIdx >= slowIdx || slowIdx >= fastIdx {
		t.Errorf("expected rows sorted by cost (loader < slow < fast), got positions %d, %d, %d\n---\n%s", loaderIdx, slowIdx, fastIdx, out)
	}
}

// A reused --write-directory must not let a previous run's report be presented
// as the current run's result.
func TestRunBenchmark_ClearsStaleReport(t *testing.T) {
	writeDir := t.TempDir()
	service := "test-service"

	reportPath := filepath.Join(writeDir, service, pluginkit.BenchmarkFileName)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		t.Fatalf("seeding report dir: %v", err)
	}
	stale := []byte(`{"schema":"privateer-benchmark/v1","plugin-name":"stale-run"}`)
	if err := os.WriteFile(reportPath, stale, 0o640); err != nil {
		t.Fatalf("seeding stale report: %v", err)
	}

	// The binary does not exist, so this run produces no report of its own.
	report, _, err := runBenchmark(filepath.Join(writeDir, "no-such-plugin"), service, writeDir, false)
	if err == nil {
		t.Fatalf("expected an error for a failed run, got report %+v", report)
	}
	if report != nil {
		t.Errorf("expected no report from a failed run, got %+v", report)
	}
	if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
		data, _ := os.ReadFile(reportPath)
		t.Errorf("stale report survived the run: %s", data)
	}
}
