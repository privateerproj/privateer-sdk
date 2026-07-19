package pluginkit

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"runtime"
	"time"

	"github.com/privateerproj/privateer-sdk/utils"
)

// BenchmarkFileName is the report's fixed name, written into
// <write-directory>/<service>/ and located by the harness after a run.
const BenchmarkFileName = "benchmark.json"

// BenchmarkSchema identifies the benchmark report format for machine consumers.
const BenchmarkSchema = "privateer-benchmark/v1"

// APICallReporter is an optional interface a plugin may implement to
// report its cumulative external API-call count.
type APICallReporter interface {
	APICallCount() int
}

type BenchmarkReport struct {
	Schema        string `json:"schema" yaml:"schema"`
	PluginName    string `json:"plugin-name" yaml:"plugin-name"`
	PluginVersion string `json:"plugin-version" yaml:"plugin-version"`
	ServiceName   string `json:"service-name" yaml:"service-name"`

	Loaders         []LoaderTiming `json:"loaders" yaml:"loaders"`
	Suites          []SuiteTiming  `json:"suites" yaml:"suites"`
	TotalDurationNs int64          `json:"total-duration-ns" yaml:"total-duration-ns"`
	APICalls        *int           `json:"api-calls,omitempty" yaml:"api-calls,omitempty"`
	WallClockNs     int64          `json:"wall-clock-ns,omitempty" yaml:"wall-clock-ns,omitempty"`
}

// LoaderTiming times one DataLoader invocation.
type LoaderTiming struct {
	// Scope is "orchestrator" or "suite:<catalog-id>".
	Scope      string `json:"scope" yaml:"scope"`
	Func       string `json:"func" yaml:"func"`
	DurationNs int64  `json:"duration-ns" yaml:"duration-ns"`
}

// SuiteTiming times one evaluation suite and its executed steps.
type SuiteTiming struct {
	CatalogId  string       `json:"catalog-id" yaml:"catalog-id"`
	DurationNs int64        `json:"duration-ns" yaml:"duration-ns"`
	Steps      []StepTiming `json:"steps" yaml:"steps"`
}

// StepTiming times one executed assessment step; steps that never ran produce no entry.
type StepTiming struct {
	ControlId     string `json:"control-id" yaml:"control-id"`
	RequirementId string `json:"requirement-id" yaml:"requirement-id"`
	StepIndex     int    `json:"step-index" yaml:"step-index"`
	Step          string `json:"step" yaml:"step"`
	Result        string `json:"result" yaml:"result"`
	DurationNs    int64  `json:"duration-ns" yaml:"duration-ns"`
}

// funcName resolves a function value's symbol name, as gemara names steps.
func funcName(fn any) string {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return "<unknown function>"
	}
	f := runtime.FuncForPC(v.Pointer())
	if f == nil {
		return "<unknown function>"
	}
	return f.Name()
}

// recordLoader appends a loader timing when benchmark mode is active.
func (v *EvaluationOrchestrator) recordLoader(scope string, loader DataLoader, d time.Duration) {
	if v.benchmark == nil {
		return
	}
	v.benchmark.Loaders = append(v.benchmark.Loaders, LoaderTiming{
		Scope:      scope,
		Func:       funcName(loader),
		DurationNs: d.Nanoseconds(),
	})
}

// apiCallsReported sums the APICallReporter counts, false when none report.
func (v *EvaluationOrchestrator) apiCallsReported() (int, bool) {
	var total int
	var found bool
	add := func(payload any) {
		if reporter, ok := payload.(APICallReporter); ok {
			total += reporter.APICallCount()
			found = true
		}
	}

	add(v.Payload)
	// possibleSuites, not Evaluation_Suites: loadPayload runs every possible
	// suite's loader, and payload-only mode leaves Evaluation_Suites empty.
	// The loader guard skips suites sharing v.Payload, so nothing double-counts.
	for _, suite := range v.possibleSuites {
		if suite.loader != nil {
			add(suite.payload)
		}
	}
	return total, found
}

// finalizeBenchmark completes and writes the report to
// <write-directory>/<service>/benchmark.json
// ignores --write=false
func (v *EvaluationOrchestrator) finalizeBenchmark(start time.Time) {
	if v.benchmark == nil {
		return
	}
	v.benchmark.ServiceName = v.ServiceName
	for _, suite := range v.Evaluation_Suites {
		v.benchmark.Suites = append(v.benchmark.Suites, SuiteTiming{
			CatalogId:  suite.CatalogId,
			DurationNs: suite.durationNs,
			Steps:      suite.stepTimings,
		})
	}

	// This makes an unreported value nil instead of zero
	if calls, ok := v.apiCallsReported(); ok {
		v.benchmark.APICalls = &calls
	}
	v.benchmark.TotalDurationNs = time.Since(start).Nanoseconds()

	data, err := json.MarshalIndent(v.benchmark, "", "  ")
	if err != nil {
		v.config.Logger.Warn("failed to marshal benchmark report", "error", err)
		return
	}
	dir := path.Join(v.config.WriteDirectory, v.ServiceName)
	if err := os.MkdirAll(dir, utils.DirPermissions); err != nil {
		v.config.Logger.Warn("failed to create benchmark report directory", "directory", dir, "error", err)
		return
	}
	filepath := path.Join(dir, BenchmarkFileName)
	if err := os.WriteFile(filepath, data, 0o640); err != nil {
		v.config.Logger.Warn("failed to write benchmark report", "filepath", filepath, "error", err)
	}
}
