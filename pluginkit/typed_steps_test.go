package pluginkit

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/config"
)

// testPayload stands in for a plugin's concrete payload type.
type testPayload struct {
	Repo string
}

// pluginTypedStep mirrors a plugin's own named step type (e.g. the scanner's
// evaluation_plans.TypedStep), to prove the ~ constraint accepts it.
type pluginTypedStep func(testPayload) (gemara.Result, string, gemara.ConfidenceLevel)

func typedStep_BranchProtection(testPayload) (gemara.Result, string, gemara.ConfidenceLevel) {
	return gemara.Passed, "branch protection ok", gemara.High
}

func typedStep_SigningEnabled(testPayload) (gemara.Result, string, gemara.ConfidenceLevel) {
	return gemara.Passed, "signing ok", gemara.High
}

// pluginAdapter reproduces the closure adapter that destroys step identity, so
// the test asserts against the exact regression being fixed.
func (s pluginTypedStep) pluginAdapter() gemara.AssessmentStep {
	return func(p any) (gemara.Result, string, gemara.ConfidenceLevel) {
		payload, ok := p.(testPayload)
		if !ok {
			return gemara.Unknown, "wrong payload", 0
		}
		return s(payload)
	}
}

// TestPluginSideAdapterCollapsesStepIdentity documents why the SDK must own the
// adaptation: a plugin-side adapter yields one symbol for every step.
func TestPluginSideAdapterCollapsesStepIdentity(t *testing.T) {
	steps := []pluginTypedStep{typedStep_BranchProtection, typedStep_SigningEnabled}

	names := map[string]bool{}
	for _, s := range steps {
		names[FuncName(s.pluginAdapter())] = true
	}
	if len(names) != 1 {
		t.Fatalf("expected the adapter to collapse both steps to one symbol, got %d: %v", len(names), names)
	}

	// the same values, named before capture, stay distinct
	direct := map[string]bool{}
	for _, s := range steps {
		direct[FuncName(s)] = true
	}
	if len(direct) != 2 {
		t.Errorf("expected 2 distinct names when resolved before capture, got %d: %v", len(direct), direct)
	}
}

func TestAdaptTypedSteps_CapturesRealNames(t *testing.T) {
	steps := map[string][]pluginTypedStep{
		"CCC.Core.C01.TR01": {typedStep_BranchProtection, typedStep_SigningEnabled},
	}

	adapted, names := adaptTypedSteps[pluginTypedStep, testPayload](steps)

	got := names["CCC.Core.C01.TR01"]
	if len(got) != 2 {
		t.Fatalf("expected 2 captured names, got %d", len(got))
	}
	if !strings.HasSuffix(got[0], "typedStep_BranchProtection") {
		t.Errorf("expected first name to be the real function, got %q", got[0])
	}
	if !strings.HasSuffix(got[1], "typedStep_SigningEnabled") {
		t.Errorf("expected second name to be the real function, got %q", got[1])
	}

	// the adapted step still runs, with the payload asserted for the caller
	result, message, _ := adapted["CCC.Core.C01.TR01"][0](testPayload{Repo: "x"})
	if result != gemara.Passed {
		t.Errorf("expected adapted step to pass, got %v (%s)", result, message)
	}
}

func TestAdaptTypedSteps_PayloadMismatch(t *testing.T) {
	steps := map[string][]pluginTypedStep{
		"CCC.Core.C01.TR01": {typedStep_BranchProtection},
	}
	adapted, _ := adaptTypedSteps[pluginTypedStep, testPayload](steps)

	result, message, _ := adapted["CCC.Core.C01.TR01"][0]("not-a-payload")
	if result != gemara.Unknown {
		t.Errorf("expected Unknown on payload mismatch, got %v", result)
	}
	if !strings.Contains(message, "expected pluginkit.testPayload, got string") {
		t.Errorf("expected a type-mismatch message naming both types, got %q", message)
	}
}

// TestBenchmark_TypedSteps_ReportsRealNames is the end-to-end assertion: a
// benchmark run of typed steps names each step by its own function.
func TestBenchmark_TypedSteps_ReportsRealNames(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = false
	cfg.WriteDirectory = tmpDir
	cfg.Benchmark = true

	typed := map[string][]pluginTypedStep{
		"CCC.Core.C01.TR01": {typedStep_BranchProtection, typedStep_SigningEnabled},
	}
	adapted, names := adaptTypedSteps[pluginTypedStep, testPayload](typed)

	orchestrator := benchmarkOrchestrator(cfg, adapted)
	orchestrator.loader = func(*config.Config) (any, error) { return testPayload{Repo: "x"}, nil }
	orchestrator.possibleSuites[0].stepNames = names

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

	var reported []string
	for _, suite := range report.Suites {
		for _, step := range suite.Steps {
			reported = append(reported, step.Step)
		}
	}
	if len(reported) != 2 {
		t.Fatalf("expected 2 step timings, got %d: %v", len(reported), reported)
	}
	for i, want := range []string{"typedStep_BranchProtection", "typedStep_SigningEnabled"} {
		if !strings.HasSuffix(reported[i], want) {
			t.Errorf("step %d: expected name ending %q, got %q", i, want, reported[i])
		}
	}
}
