package pluginkit

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/gemaraproj/go-gemara"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/shared"
)

// countingPayload implements APICallReporter for benchmark tests.
type countingPayload struct {
	calls int
}

func (p *countingPayload) APICallCount() int { return p.calls }

func slowLoader(_ *config.Config) (any, error) {
	time.Sleep(2 * time.Millisecond)
	return &countingPayload{calls: 7}, nil
}

func step_Slow(_ interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
	time.Sleep(5 * time.Millisecond)
	return gemara.Passed, "This step is slow but passes", gemara.High
}

// benchmarkOrchestrator builds a manually-constructed orchestrator matching the
// Mobilize test pattern, with benchmark mode toggled by the caller.
func benchmarkOrchestrator(cfg *config.Config, steps map[string][]gemara.AssessmentStep) *EvaluationOrchestrator {
	catalog := getTestCatalogWithRequirements()
	return &EvaluationOrchestrator{
		ServiceName: "test-service",
		PluginName:  "test-plugin",
		config:      cfg,
		loader:      slowLoader,
		possibleSuites: []*EvaluationSuite{
			{CatalogId: "CCC.ObjStor", catalog: catalog, steps: steps, config: cfg},
		},
	}
}

func TestBenchmark_Mobilize_WritesReport(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false // the report must be written even when results are not
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true

	steps := map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Pass, step_Slow},
	}
	orchestrator := benchmarkOrchestrator(cfg, steps)

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	reportPath := path.Join(tmpDir, "test-service", BenchmarkFileName)
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected benchmark report at %s: %v", reportPath, err)
	}

	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("benchmark report is not valid JSON: %v", err)
	}

	if report.Schema != BenchmarkSchema {
		t.Errorf("expected schema %q, got %q", BenchmarkSchema, report.Schema)
	}
	if report.PluginName != "test-plugin" || report.ServiceName != "test-service" {
		t.Errorf("expected plugin/service identity, got %q/%q", report.PluginName, report.ServiceName)
	}

	if len(report.Loaders) != 1 {
		t.Fatalf("expected 1 loader timing, got %d", len(report.Loaders))
	}
	loader := report.Loaders[0]
	if loader.Scope != "orchestrator" {
		t.Errorf("expected loader scope 'orchestrator', got %q", loader.Scope)
	}
	if loader.DurationNs < (2 * time.Millisecond).Nanoseconds() {
		t.Errorf("expected loader duration >= 2ms, got %dns", loader.DurationNs)
	}
	if !strings.Contains(loader.Func, "slowLoader") {
		t.Errorf("expected loader func name to contain 'slowLoader', got %q", loader.Func)
	}

	if len(report.Suites) != 1 {
		t.Fatalf("expected 1 suite timing, got %d", len(report.Suites))
	}
	suite := report.Suites[0]
	if suite.CatalogId != "CCC.ObjStor" {
		t.Errorf("expected suite catalog CCC.ObjStor, got %q", suite.CatalogId)
	}
	if suite.DurationNs <= 0 {
		t.Errorf("expected positive suite duration, got %dns", suite.DurationNs)
	}
	if len(suite.Steps) != 2 {
		t.Fatalf("expected 2 executed step timings, got %d", len(suite.Steps))
	}
	for _, st := range suite.Steps {
		if st.ControlId != "CCC.Core.C01" || st.RequirementId != "CCC.Core.C01.TR01" {
			t.Errorf("step timing misattributed: %+v", st)
		}
		if st.Result != "Passed" {
			t.Errorf("expected step result Passed, got %q", st.Result)
		}
		if st.DurationNs <= 0 {
			t.Errorf("expected positive step duration, got %dns", st.DurationNs)
		}
	}
	if !strings.Contains(suite.Steps[0].Step, "step_Pass") || !strings.Contains(suite.Steps[1].Step, "step_Slow") {
		t.Errorf("expected original step names to be preserved, got %q, %q", suite.Steps[0].Step, suite.Steps[1].Step)
	}
	// The 5ms sleeper must read as such at sub-ms resolution — the whole point
	// of monotonic durations over the second-resolution timestamps they replace.
	if suite.Steps[1].DurationNs < (5 * time.Millisecond).Nanoseconds() {
		t.Errorf("expected slow step >= 5ms, got %dns", suite.Steps[1].DurationNs)
	}

	if report.APICalls == nil || *report.APICalls != 7 {
		t.Errorf("expected api-calls 7 from APICallReporter payload, got %v", report.APICalls)
	}
	if report.TotalDurationNs < loader.DurationNs {
		t.Errorf("expected total (%dns) >= loader (%dns)", report.TotalDurationNs, loader.DurationNs)
	}

	// StartTime must now be parseable, high-resolution RFC3339.
	if _, err := time.Parse(time.RFC3339Nano, orchestrator.Evaluation_Suites[0].StartTime); err != nil {
		t.Errorf("suite StartTime is not RFC3339Nano: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, orchestrator.Evaluation_Suites[0].EndTime); err != nil {
		t.Errorf("suite EndTime is not RFC3339Nano: %v", err)
	}
}

// Payload-only mode times the loader and stops: the assessment steps must not
// run, and the report must carry the loader timing but no suites.
func TestBenchmark_PayloadOnly_SkipsAssessment(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true
	cfg.BenchmarkPayloadOnly = true

	stepRan := false
	sentinel := func(_ interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
		stepRan = true
		return gemara.Passed, "should never run in payload-only mode", gemara.High
	}
	steps := map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {sentinel},
	}
	orchestrator := benchmarkOrchestrator(cfg, steps)

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}
	if stepRan {
		t.Error("assessment step ran in payload-only mode; it must be skipped")
	}
	if len(orchestrator.Evaluation_Suites) != 0 {
		t.Errorf("expected no evaluated suites in payload-only mode, got %d", len(orchestrator.Evaluation_Suites))
	}

	reportPath := path.Join(tmpDir, "test-service", BenchmarkFileName)
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected benchmark report at %s: %v", reportPath, err)
	}
	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("benchmark report is not valid JSON: %v", err)
	}
	if len(report.Loaders) != 1 {
		t.Fatalf("expected the loader to still be timed, got %d loader timings", len(report.Loaders))
	}
	if len(report.Suites) != 0 {
		t.Errorf("expected no suite timings in payload-only mode, got %d", len(report.Suites))
	}
}

// Payload-only is inert without benchmark mode: a stray env var must never make
// a normal run skip its assessments.
func TestBenchmark_PayloadOnly_IgnoredWhenBenchmarkOff(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = false
	cfg.BenchmarkPayloadOnly = true

	stepRan := false
	sentinel := func(_ interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
		stepRan = true
		return gemara.Passed, "runs because benchmark is off", gemara.High
	}
	steps := map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {sentinel},
	}
	orchestrator := benchmarkOrchestrator(cfg, steps)

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}
	if !stepRan {
		t.Error("assessment step was skipped even though benchmark mode is off")
	}
}

// A benchmark run also writes a normal results document. The step wrapping
// must not leak into it: gemara names steps by runtime symbol, and every
// timing closure shares one symbol, so unrestored wrapping would collapse
// every assessment's steps to the same meaningless name.
func TestBenchmark_ResultsDocumentKeepsRealStepNames(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = true
	cfg.WriteDirectory = tmpDir
	cfg.Output = "json"
	cfg.Benchmark = true

	steps := map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Pass, step_Slow},
	}
	orchestrator := benchmarkOrchestrator(cfg, steps)

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	// Assert on the in-memory log and the written document: the written file is
	// what a plugin author (and any downstream consumer) actually reads.
	assessment := orchestrator.Evaluation_Suites[0].EvaluationLog.Evaluations[0].AssessmentLogs[0]
	if len(assessment.Steps) != 2 {
		t.Fatalf("expected 2 steps restored, got %d", len(assessment.Steps))
	}
	first, second := assessment.Steps[0].String(), assessment.Steps[1].String()
	if !strings.Contains(first, "step_Pass") || !strings.Contains(second, "step_Slow") {
		t.Errorf("expected the plugin's own step names, got %q and %q", first, second)
	}
	if first == second {
		t.Errorf("distinct steps collapsed to one name: %q", first)
	}

	written, err := os.ReadFile(path.Join(tmpDir, "test-service", "test-service.json"))
	if err != nil {
		t.Fatalf("reading written results: %v", err)
	}
	if strings.Contains(string(written), "timedSteps") {
		t.Error("benchmark timing closure leaked into the written results document")
	}
}

// The api-calls hook must work for suites that load their own payload, not
// only the shared orchestrator payload — per-suite loaders are a supported
// configuration and are just as likely to be API-bound.
func TestBenchmark_APICallReporter_SuiteLoader(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true

	catalog := getTestCatalogWithRequirements()
	orchestrator := &EvaluationOrchestrator{
		ServiceName: "test-service",
		PluginName:  "test-plugin",
		config:      cfg,
		// No orchestrator-level loader: the suite loads its own payload.
		possibleSuites: []*EvaluationSuite{
			{
				CatalogId: "CCC.ObjStor",
				catalog:   catalog,
				steps:     createPassingStepsMap(),
				config:    cfg,
				loader:    func(_ *config.Config) (any, error) { return &countingPayload{calls: 13}, nil },
			},
		},
	}

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	data, err := os.ReadFile(path.Join(tmpDir, "test-service", BenchmarkFileName))
	if err != nil {
		t.Fatalf("expected benchmark report: %v", err)
	}
	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("benchmark report is not valid JSON: %v", err)
	}

	if len(report.Loaders) != 1 || report.Loaders[0].Scope != "suite:CCC.ObjStor" {
		t.Errorf("expected one suite-scoped loader timing, got %+v", report.Loaders)
	}
	if report.APICalls == nil || *report.APICalls != 13 {
		t.Errorf("expected api-calls 13 from the suite payload, got %v", report.APICalls)
	}
}

func TestBenchmark_Disabled_NoReportNoWrapping(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = false

	orchestrator := benchmarkOrchestrator(cfg, createPassingStepsMap())

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	reportPath := path.Join(tmpDir, "test-service", BenchmarkFileName)
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Errorf("expected no benchmark report when benchmark mode is off, stat err: %v", err)
	}

	suite := orchestrator.Evaluation_Suites[0]
	if suite.stepTimings != nil {
		t.Errorf("expected no step timings when benchmark mode is off, got %d", len(suite.stepTimings))
	}
	// The serialized step name must be the plugin's own function, not a wrapper:
	// normal runs must be completely untouched by the instrumentation.
	stepName := suite.EvaluationLog.Evaluations[0].AssessmentLogs[0].Steps[0].String()
	if !strings.Contains(stepName, "step_Pass") {
		t.Errorf("expected unwrapped step name containing 'step_Pass', got %q", stepName)
	}
}

func TestBenchmark_FailedStep_RecordsResultAndHalts(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true

	steps := map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Fail, step_Pass}, // halt after the failure
	}
	orchestrator := benchmarkOrchestrator(cfg, steps)

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	data, err := os.ReadFile(path.Join(tmpDir, "test-service", BenchmarkFileName))
	if err != nil {
		t.Fatalf("expected benchmark report: %v", err)
	}
	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("benchmark report is not valid JSON: %v", err)
	}

	if len(report.Suites) != 1 || len(report.Suites[0].Steps) != 1 {
		t.Fatalf("expected exactly the executed (failing) step to be timed, got %+v", report.Suites)
	}
	st := report.Suites[0].Steps[0]
	if st.Result != "Failed" || !strings.Contains(st.Step, "step_Fail") {
		t.Errorf("expected the failing step's timing, got %+v", st)
	}
}

// Payload-only mode exists to isolate loader cost, so it must still report the
// API calls of a per-suite loader's payload even though no suite is evaluated.
func TestBenchmark_PayloadOnly_ReportsPerSuiteAPICalls(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true
	cfg.BenchmarkPayloadOnly = true

	catalog := getTestCatalogWithRequirements()
	orchestrator := &EvaluationOrchestrator{
		ServiceName: "test-service",
		PluginName:  "test-plugin",
		config:      cfg,
		// no orchestrator loader: the payload comes only from the suite
		possibleSuites: []*EvaluationSuite{
			{
				CatalogId: "CCC.ObjStor",
				catalog:   catalog,
				steps:     map[string][]gemara.AssessmentStep{"CCC.Core.C01.TR01": {step_Pass}},
				config:    cfg,
				loader: func(*config.Config) (any, error) {
					return &countingPayload{calls: 7}, nil
				},
			},
		},
	}

	if err := orchestrator.Mobilize(); err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	data, err := os.ReadFile(path.Join(tmpDir, "test-service", BenchmarkFileName))
	if err != nil {
		t.Fatalf("expected benchmark report: %v", err)
	}
	var report BenchmarkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("benchmark report is not valid JSON: %v", err)
	}

	if report.APICalls == nil {
		t.Fatalf("expected the per-suite payload's API calls to be reported, got nil")
	}
	if *report.APICalls != 7 {
		t.Errorf("expected 7 API calls, got %d", *report.APICalls)
	}
}

// A benchmark report that cannot be written must fail the run rather than be
// swallowed: the plugin's log lands in a file the harness never reads, so a
// silent failure leaves the harness guessing why the report is missing.
func TestBenchmark_UnwritableReport_FailsMobilize(t *testing.T) {
	tmpDir := t.TempDir()

	// Occupy the report's directory path with a regular file so MkdirAll fails.
	blocker := path.Join(tmpDir, "test-service")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o640); err != nil {
		t.Fatalf("seeding blocker: %v", err)
	}

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false // isolate the benchmark write from result writing
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true

	orchestrator := benchmarkOrchestrator(cfg, map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Pass},
	})

	err := orchestrator.Mobilize()
	if err == nil {
		t.Fatal("expected Mobilize to fail when the benchmark report cannot be written")
	}
	if !strings.Contains(err.Error(), "failed to write benchmark report") {
		t.Errorf("expected a benchmark-write diagnosis, got: %v", err)
	}
	// ErrRuntime, not ErrDevBug: a full disk is not plugin misuse.
	if !errors.Is(err, ErrRuntime) {
		t.Errorf("expected ErrRuntime so ExitCodeFor yields InternalError, got: %v", err)
	}
	if code := ExitCodeFor(orchestrator, err); code != shared.InternalError {
		t.Errorf("expected InternalError so the harness reports the real cause, got %d", code)
	}
}
