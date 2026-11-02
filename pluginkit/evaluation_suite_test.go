package pluginkit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
	hclog "github.com/hashicorp/go-hclog"
)

func BenchmarkEvaluate_Passing(b *testing.B) {
	cfg := setBasicConfig()
	steps := createPassingStepsMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		suite := &EvaluationSuite{
			Name:    "bench",
			catalog: getTestCatalogWithRequirements(),
			steps:   steps,
			config:  cfg,
		}
		b.StartTimer()
		if err := suite.Evaluate("bench-service"); err != nil {
			b.Fatalf("Evaluate failed: %v", err)
		}
	}
}

func BenchmarkSetupEvalLog(b *testing.B) {
	suite := &EvaluationSuite{
		catalog:   getTestCatalogWithRequirements(),
		CatalogId: "CCC.ObjStor",
		config:    setBasicConfig(),
	}
	steps := createPassingStepsMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.setupEvalLog(steps)
		if err != nil {
			b.Fatalf("setupEvalLog failed: %v", err)
		}
	}
}

func BenchmarkGetAssessmentRequirements(b *testing.B) {
	suite := &EvaluationSuite{
		catalog: getTestCatalogWithRequirements(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.GetAssessmentRequirements()
		if err != nil {
			b.Fatalf("GetAssessmentRequirements failed: %v", err)
		}
	}
}

func TestEvaluate(t *testing.T) {
	testData := getTestEvaluateData()

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			// Use catalog with requirements so flow reaches setupEvalLog and expected errors (e.g. sel10) or success
			catalog := getTestCatalogWithRequirements()

			suite := &EvaluationSuite{
				Name:          test.testName,
				EvaluationLog: gemara.EvaluationLog{Evaluations: test.evals},
				catalog:       catalog,
				steps:         test.steps,
			}
			suite.config = setBasicConfig()

			err := suite.Evaluate("arbitrarySuiteName")
			if err != nil && test.expectedEvalSuiteError != nil {
				if !strings.Contains(err.Error(), test.expectedEvalSuiteError.Error()) {
					t.Errorf("Expected error containing '%s', but got '%v'", test.expectedEvalSuiteError, err)
				}
			} else if err != nil && test.expectedEvalSuiteError == nil {
				// For now, we expect an error about missing assessment requirements when catalog is empty
				// This is expected behavior with the current implementation
				if !strings.Contains(err.Error(), "assessment steps not provided") {
					t.Errorf("Expected error mentioning 'assessment steps not provided', but got '%v'", err)
				}
			} else if err == nil && test.expectedEvalSuiteError != nil {
				t.Errorf("Expected error '%s', but got no error", test.expectedEvalSuiteError)
			}
		})
	}
}

func TestEvaluateWithEmptyCatalog(t *testing.T) {
	t.Run("Empty Catalog", func(t *testing.T) {
		suite := &EvaluationSuite{
			Name:    "Empty Catalog Test",
			catalog: getEmptyTestCatalog(),
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()

		err := suite.Evaluate("testService")
		if err == nil {
			t.Error("Expected error for empty catalog, but got none")
		}
		if !strings.Contains(err.Error(), "assessment requirements not provided") {
			t.Errorf("Expected 'assessment requirements not provided' error, but got: %v", err)
		}
	})

	t.Run("Catalog With No Requirements", func(t *testing.T) {
		suite := &EvaluationSuite{
			Name:    "No Requirements Test",
			catalog: getTestCatalogWithNoRequirements(),
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()

		err := suite.Evaluate("testService")
		if err == nil {
			t.Error("Expected error for catalog with no requirements, but got none")
		}
		if !strings.Contains(err.Error(), "assessment requirements not provided") {
			t.Errorf("Expected 'assessment requirements not provided' error, but got: %v", err)
		}
	})
}

func TestEvaluateWithNilConfig(t *testing.T) {
	t.Run("Nil Config", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			Name:    "Nil Config Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		// Don't set config - leave it nil

		err := suite.Evaluate("testService")
		if err == nil {
			t.Error("Expected error for nil config, but got none")
		}
		if !strings.Contains(err.Error(), "configuration not initialized") {
			t.Errorf("Expected 'configuration not initialized' error, but got: %v", err)
		}
	})
}

func TestSingleLine(t *testing.T) {
	multiline := "verdict=fail confidence=medium\nreasoning: tests are documented\ncitations: README"
	want := "verdict=fail confidence=medium reasoning: tests are documented citations: README"
	if got := singleLine(multiline); got != want {
		t.Errorf("singleLine() = %q, want %q", got, want)
	}
	if got := singleLine("already one line"); got != "already one line" {
		t.Errorf("singleLine() altered a single-line message: %q", got)
	}
}

func TestGetAssessmentRequirements(t *testing.T) {
	t.Run("Valid Catalog", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			catalog: catalog,
		}

		requirements, err := suite.GetAssessmentRequirements()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if requirements == nil {
			t.Error("Expected requirements map, but got nil")
		}
		if len(requirements) == 0 {
			t.Error("Expected non-empty requirements map")
		}
	})

	t.Run("Empty Catalog", func(t *testing.T) {
		suite := &EvaluationSuite{
			catalog: getEmptyTestCatalog(),
		}

		requirements, err := suite.GetAssessmentRequirements()
		if err == nil {
			t.Error("Expected error for empty catalog, but got none")
		}
		if !strings.Contains(err.Error(), "assessment requirements not provided") {
			t.Errorf("Expected 'assessment requirements not provided' error, but got: %v", err)
		}
		if requirements != nil {
			t.Error("Expected nil requirements for empty catalog")
		}
	})

	t.Run("Catalog With No Requirements", func(t *testing.T) {
		suite := &EvaluationSuite{
			catalog: getTestCatalogWithNoRequirements(),
		}

		requirements, err := suite.GetAssessmentRequirements()
		if err == nil {
			t.Error("Expected error for catalog with no requirements, but got none")
		}
		if !strings.Contains(err.Error(), "assessment requirements not provided") {
			t.Errorf("Expected 'assessment requirements not provided' error, but got: %v", err)
		}
		if requirements != nil {
			t.Error("Expected nil requirements for catalog with no requirements")
		}
	})
}

func TestSetupEvalLog(t *testing.T) {
	t.Run("Valid Steps Map", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			catalog: catalog,
		}

		steps := createPassingStepsMap()
		evalLog, err := suite.setupEvalLog(steps)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(evalLog.Evaluations) == 0 {
			t.Error("Expected non-empty evaluations list")
		}
	})

	t.Run("Assessment requirements cite the catalog", func(t *testing.T) {
		// Gemara unifies each assessment-log's requirement reference-id with its
		// control's, and the formulation supplies the control's value only when
		// the assessment OMITS one. An unset ReferenceId is not omitted — it
		// serializes as "" and contradicts the control's, so a log that leaves
		// it unset fails validation against gemara's own schema.
		suite := &EvaluationSuite{
			catalog:   getTestCatalogWithRequirements(),
			CatalogId: "CCC.ObjStor",
		}

		evalLog, err := suite.setupEvalLog(createPassingStepsMap())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(evalLog.Evaluations) == 0 {
			t.Fatal("Expected non-empty evaluations list")
		}

		for _, evaluation := range evalLog.Evaluations {
			if evaluation.Control.ReferenceId != suite.CatalogId {
				t.Errorf("control reference-id = %q, want %q",
					evaluation.Control.ReferenceId, suite.CatalogId)
			}
			if len(evaluation.AssessmentLogs) == 0 {
				t.Errorf("control %q produced no assessment logs", evaluation.Control.EntryId)
			}
			for _, assessment := range evaluation.AssessmentLogs {
				if assessment.Requirement.ReferenceId != suite.CatalogId {
					t.Errorf("requirement %q reference-id = %q, want %q — an unset value serializes as \"\" and conflicts with the control's",
						assessment.Requirement.EntryId,
						assessment.Requirement.ReferenceId,
						suite.CatalogId)
				}
			}
		}
	})

	t.Run("Empty Steps Map", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			catalog: catalog,
		}

		steps := map[string][]gemara.AssessmentStep{}
		evalLog, err := suite.setupEvalLog(steps)
		if err == nil {
			t.Error("Expected error for empty steps map, but got none")
		}
		if !strings.Contains(err.Error(), "assessment steps not provided") {
			t.Errorf("Expected 'assessment steps not provided' error, but got: %v", err)
		}
		if len(evalLog.Evaluations) != 0 {
			t.Error("Expected empty evaluations list for empty steps")
		}
	})

	t.Run("Nil Steps Map", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			catalog: catalog,
		}

		evalLog, err := suite.setupEvalLog(nil)
		if err == nil {
			t.Error("Expected error for nil steps map, but got none")
		}
		if !strings.Contains(err.Error(), "assessment steps not provided") {
			t.Errorf("Expected 'assessment steps not provided' error, but got: %v", err)
		}
		if len(evalLog.Evaluations) != 0 {
			t.Error("Expected empty evaluations list for nil steps")
		}
	})

	t.Run("Empty Catalog", func(t *testing.T) {
		suite := &EvaluationSuite{
			catalog: getEmptyTestCatalog(),
		}

		steps := createPassingStepsMap()
		evalLog, err := suite.setupEvalLog(steps)
		if err == nil {
			t.Error("Expected error for empty catalog, but got none")
		}
		if !strings.Contains(err.Error(), "evaluation suite crashed") {
			t.Errorf("Expected 'evaluation suite crashed' error, but got: %v", err)
		}
		if len(evalLog.Evaluations) != 0 {
			t.Error("Expected empty evaluations list for empty catalog")
		}
	})

	t.Run("Catalog With No Requirements", func(t *testing.T) {
		suite := &EvaluationSuite{
			catalog: getTestCatalogWithNoRequirements(),
		}

		steps := createPassingStepsMap()
		evalLog, err := suite.setupEvalLog(steps)
		if err == nil {
			t.Error("Expected error for catalog with no requirements, but got none")
		}
		if !strings.Contains(err.Error(), "evaluation suite crashed") {
			t.Errorf("Expected 'evaluation suite crashed' error, but got: %v", err)
		}
		if len(evalLog.Evaluations) != 0 {
			t.Error("Expected empty evaluations list for catalog with no requirements")
		}
	})
}

func TestAddChangeManager(t *testing.T) {
	t.Run("Invasive Config With Change Manager", func(t *testing.T) {
		suite := &EvaluationSuite{}
		suite.config = setBasicConfig()
		suite.config.Invasive = true

		changeManager := &ChangeManager{}
		suite.AddChangeManager(changeManager)

		if suite.changeManager == nil {
			t.Error("Expected change manager to be set")
		}
		if !suite.changeManager.Allowed {
			t.Error("Expected change manager to be allowed")
		}
	})

	t.Run("Non-Invasive Config With Change Manager", func(t *testing.T) {
		suite := &EvaluationSuite{}
		suite.config = setBasicConfig()
		suite.config.Invasive = false

		changeManager := &ChangeManager{}
		suite.AddChangeManager(changeManager)

		if suite.changeManager != nil {
			t.Error("Expected change manager to remain nil for non-invasive config")
		}
	})

	t.Run("Invasive Config With Nil Change Manager", func(t *testing.T) {
		suite := &EvaluationSuite{}
		suite.config = setBasicConfig()
		suite.config.Invasive = true

		suite.AddChangeManager(nil)

		if suite.changeManager != nil {
			t.Error("Expected change manager to remain nil when nil is passed")
		}
	})

	t.Run("Nil Config With Change Manager", func(t *testing.T) {
		suite := &EvaluationSuite{}
		// Don't set config - leave it nil

		changeManager := &ChangeManager{}

		// This should panic due to nil config, so we need to recover
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when config is nil, but no panic occurred")
			}
		}()

		suite.AddChangeManager(changeManager)
	})
}

func TestEvaluationSuiteIntegration(t *testing.T) {
	t.Run("Complete Evaluation Flow", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			Name:    "Integration Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()

		err := suite.Evaluate("integrationTest")
		if err != nil {
			t.Errorf("Unexpected error in integration test: %v", err)
		}

		// Verify that the suite was properly configured
		if suite.Name == "" {
			t.Error("Expected suite name to be set")
		}
		if suite.StartTime == "" {
			t.Error("Expected start time to be set")
		}
		if suite.EndTime == "" {
			t.Error("Expected end time to be set")
		}
	})

	t.Run("Evaluation With Change Manager", func(t *testing.T) {
		catalog := getTestCatalogWithRequirements()

		suite := &EvaluationSuite{
			Name:    "Change Manager Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()
		suite.config.Invasive = true

		changeManager := &ChangeManager{}
		suite.AddChangeManager(changeManager)

		err := suite.Evaluate("changeManagerTest")
		if err != nil {
			t.Errorf("Unexpected error in change manager test: %v", err)
		}

		// Verify change manager was properly set up
		if suite.changeManager == nil {
			t.Error("Expected change manager to be set")
		}
	})
}

func TestEvaluationSuite_RetiredRequirementsAndControls(t *testing.T) {
	t.Run("Retired requirement ignores mapped steps and records NotRun", func(t *testing.T) {
		stepExecuted := false
		failingStep := func(payload interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
			stepExecuted = true
			return gemara.Failed, "should not be called", gemara.High
		}
		passingStep := func(payload interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
			return gemara.Passed, "active check passed", gemara.High
		}

		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{
				{
					Id:        "SEC-01",
					Title:     "Security Control",
					Objective: "Ensure security",
					AssessmentRequirements: []gemara.AssessmentRequirement{
						{
							Id:            "SEC-01.01",
							Text:          "Active requirement",
							Applicability: requestedApplicability,
							State:         gemara.LifecycleActive,
						},
						{
							Id:            "SEC-01.02",
							Text:          "Retired requirement",
							Applicability: requestedApplicability,
							State:         gemara.LifecycleRetired,
							ReplacedBy: &gemara.EntryMapping{
								EntryId: "SEC-01.03",
							},
						},
					},
				},
			},
		}

		suite := &EvaluationSuite{
			Name:    "Retired Requirement Test",
			catalog: catalog,
			steps: map[string][]gemara.AssessmentStep{
				"SEC-01.01": {passingStep},
				"SEC-01.02": {failingStep}, // Mapped step should be ignored!
			},
		}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		if stepExecuted {
			t.Errorf("mapped step for retired requirement was executed")
		}

		// Suite and control results should be Passed because active requirement passed and retired is NotRun
		if suite.Result != gemara.Passed {
			t.Errorf("suite.Result = %v, want Passed", suite.Result)
		}

		// Check the individual assessment logs
		evals := suite.EvaluationLog.Evaluations
		if len(evals) != 1 || len(evals[0].AssessmentLogs) != 2 {
			t.Fatalf("unexpected evaluations structure: %+v", evals)
		}

		activeLog := evals[0].AssessmentLogs[0]
		retiredLog := evals[0].AssessmentLogs[1]

		if activeLog.Result != gemara.Passed {
			t.Errorf("activeLog.Result = %v, want Passed", activeLog.Result)
		}
		if retiredLog.Result != gemara.NotRun {
			t.Errorf("retiredLog.Result = %v, want NotRun", retiredLog.Result)
		}
		if !strings.Contains(retiredLog.Message, "SEC-01.02 is retired (replaced by SEC-01.03)") {
			t.Errorf("retiredLog.Message = %q, want retirement and replaced-by mention", retiredLog.Message)
		}

		// Check scoring metrics: only 1 success, 0 failures, 0 warnings
		if suite.evalSuccesses != 1 {
			t.Errorf("evalSuccesses = %d, want 1", suite.evalSuccesses)
		}
		if suite.evalFailures != 0 {
			t.Errorf("evalFailures = %d, want 0", suite.evalFailures)
		}
		if suite.evalWarnings != 0 {
			t.Errorf("evalWarnings = %d, want 0", suite.evalWarnings)
		}
	})

	t.Run("Retired requirement without mapped steps does not log warning or set Unknown", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{
				{
					Id:        "SEC-02",
					Title:     "Security Control 2",
					Objective: "Ensure security",
					AssessmentRequirements: []gemara.AssessmentRequirement{
						{
							Id:            "SEC-02.01",
							Text:          "Retired requirement without mapped steps",
							Applicability: requestedApplicability,
							State:         gemara.LifecycleRetired,
						},
					},
				},
			},
		}

		passingStep := func(payload interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
			return gemara.Passed, "unrelated step", gemara.High
		}

		suite := &EvaluationSuite{
			Name:    "Retired Requirement No Steps Test",
			catalog: catalog,
			steps: map[string][]gemara.AssessmentStep{
				"OTHER.REQ": {passingStep}, // SEC-02.01 has NO mapped steps
			},
		}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		evals := suite.EvaluationLog.Evaluations
		if len(evals) != 1 || len(evals[0].AssessmentLogs) != 1 {
			t.Fatalf("unexpected evaluations structure: %+v", evals)
		}

		log := evals[0].AssessmentLogs[0]
		if log.Result != gemara.NotRun {
			t.Errorf("log.Result = %v, want NotRun (must not be Unknown)", log.Result)
		}
		if !strings.Contains(log.Message, "requirement SEC-02.01 is retired") {
			t.Errorf("log.Message = %q, want retired requirement message", log.Message)
		}
	})

	t.Run("Entire control retired marks all requirements NotRun", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{
				{
					Id:        "LEGACY-01",
					Title:     "Legacy Control",
					Objective: "Old objective",
					State:     gemara.LifecycleRetired,
					ReplacedBy: &gemara.EntryMapping{
						EntryId: "MODERN-01",
					},
					AssessmentRequirements: []gemara.AssessmentRequirement{
						{
							Id:            "LEGACY-01.01",
							Text:          "Legacy requirement",
							Applicability: requestedApplicability,
						},
					},
				},
			},
		}

		failingStep := func(payload interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
			return gemara.Failed, "should not be called", gemara.High
		}

		suite := &EvaluationSuite{
			Name:    "Retired Control Test",
			catalog: catalog,
			steps: map[string][]gemara.AssessmentStep{
				"LEGACY-01.01": {failingStep},
			},
		}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		evals := suite.EvaluationLog.Evaluations
		if len(evals) != 1 || len(evals[0].AssessmentLogs) != 1 {
			t.Fatalf("unexpected evaluations structure: %+v", evals)
		}

		log := evals[0].AssessmentLogs[0]
		if log.Result != gemara.NotRun {
			t.Errorf("log.Result = %v, want NotRun", log.Result)
		}
		if !strings.Contains(log.Message, "control LEGACY-01 is retired (replaced by MODERN-01)") {
			t.Errorf("log.Message = %q, want control retirement message", log.Message)
		}
	})
}

func step_NotRunNoMessage(_ interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
	return gemara.NotRun, "", gemara.High
}

func TestEvaluationSuite_RetiredReviewFollowUps(t *testing.T) {
	outsideApplicability := []string{"tlp-red"}

	t.Run("Benchmark restores real step names for non-retired NotRun and inapplicable assessments", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "SEC-10", Title: "Control", Objective: "Objective",
				AssessmentRequirements: []gemara.AssessmentRequirement{
					{Id: "SEC-10.01", Text: "Step returns NotRun", Applicability: requestedApplicability},
					{Id: "SEC-10.02", Text: "Outside applicability", Applicability: outsideApplicability},
				},
			}},
		}
		suite := &EvaluationSuite{
			Name:    "Benchmark NotRun Test",
			catalog: catalog,
			steps: map[string][]gemara.AssessmentStep{
				"SEC-10.01": {step_NotRunNoMessage},
				"SEC-10.02": {step_Pass},
			},
		}
		suite.config = setBasicConfig()
		suite.config.Benchmark = true

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		logs := suite.EvaluationLog.Evaluations[0].AssessmentLogs
		want := map[string]string{"SEC-10.01": "step_NotRunNoMessage", "SEC-10.02": "step_Pass"}
		for _, log := range logs {
			if len(log.Steps) != 1 {
				t.Fatalf("%s: expected 1 step, got %d", log.Requirement.EntryId, len(log.Steps))
			}
			name := log.Steps[0].String()
			if strings.Contains(name, "timedSteps") || !strings.Contains(name, want[log.Requirement.EntryId]) {
				t.Errorf("%s: step name = %q, want the plugin's %s", log.Requirement.EntryId, name, want[log.Requirement.EntryId])
			}
		}
	})

	t.Run("Retired requirement outside requested applicability keeps the retirement message", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "SEC-11", Title: "Control", Objective: "Objective",
				AssessmentRequirements: []gemara.AssessmentRequirement{
					{Id: "SEC-11.01", Text: "Active", Applicability: requestedApplicability},
					{
						Id: "SEC-11.02", Text: "Retired, not applicable", Applicability: outsideApplicability,
						State: gemara.LifecycleRetired, ReplacedBy: &gemara.EntryMapping{EntryId: "SEC-11.03"},
					},
				},
			}},
		}
		suite := &EvaluationSuite{
			Name:    "Retired Inapplicable Test",
			catalog: catalog,
			steps:   map[string][]gemara.AssessmentStep{"SEC-11.01": {step_Pass}},
		}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		retiredLog := suite.EvaluationLog.Evaluations[0].AssessmentLogs[1]
		if retiredLog.Result != gemara.NotRun {
			t.Errorf("retiredLog.Result = %v, want NotRun", retiredLog.Result)
		}
		if retiredLog.Message != "requirement SEC-11.02 is retired (replaced by SEC-11.03)" {
			t.Errorf("retiredLog.Message = %q, want the retirement message with ReplacedBy", retiredLog.Message)
		}
	})

	t.Run("Retired control names the control, not the requirement, as retired", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "LEGACY-02", Title: "Legacy", Objective: "Old",
				State:      gemara.LifecycleRetired,
				ReplacedBy: &gemara.EntryMapping{EntryId: "MODERN-02"},
				AssessmentRequirements: []gemara.AssessmentRequirement{{
					Id: "LEGACY-02.01", Text: "Not itself retired", Applicability: requestedApplicability,
					ReplacedBy: &gemara.EntryMapping{EntryId: "MODERN-02.01"},
				}},
			}},
		}
		suite := &EvaluationSuite{
			Name:    "Retired Control Message Test",
			catalog: catalog,
			steps:   map[string][]gemara.AssessmentStep{"LEGACY-02.01": {step_Pass}},
		}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		got := suite.EvaluationLog.Evaluations[0].AssessmentLogs[0].Message
		want := "control LEGACY-02 is retired (replaced by MODERN-02); requirement LEGACY-02.01 is replaced by MODERN-02.01"
		if got != want {
			t.Errorf("Message = %q, want %q", got, want)
		}
	})

	t.Run("NotRun assessment with an empty message is not logged", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "SEC-12", Title: "Control", Objective: "Objective",
				AssessmentRequirements: []gemara.AssessmentRequirement{
					{Id: "SEC-12.01", Text: "Step returns NotRun", Applicability: requestedApplicability},
					{Id: "SEC-12.02", Text: "Retired", Applicability: requestedApplicability, State: gemara.LifecycleRetired},
				},
			}},
		}
		suite := &EvaluationSuite{
			Name:    "Empty NotRun Log Test",
			catalog: catalog,
			steps:   map[string][]gemara.AssessmentStep{"SEC-12.01": {step_NotRunNoMessage}},
		}
		suite.config = setBasicConfig()
		var buf bytes.Buffer
		suite.config.Logger = hclog.New(&hclog.LoggerOptions{Output: &buf, Level: hclog.Info})

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		out := buf.String()
		if strings.Contains(out, "SEC-12.01:") {
			t.Errorf("bare NotRun line was logged:\n%s", out)
		}
		if !strings.Contains(out, "SEC-12.02: requirement SEC-12.02 is retired") {
			t.Errorf("retired requirement was not logged:\n%s", out)
		}
	})
}

func TestEvaluationSuite_RetiredSecondReview(t *testing.T) {
	outsideApplicability := []string{"tlp-red"}

	for _, tc := range []struct {
		name          string
		applicability []string
	}{
		{"outside requested applicability", outsideApplicability},
		{"inside requested applicability", requestedApplicability},
	} {
		t.Run("control whose only requirement is retired is NotRun, "+tc.name, func(t *testing.T) {
			catalog := &gemara.ControlCatalog{
				Metadata: gemara.Metadata{Id: "test-catalog"},
				Controls: []gemara.Control{{
					Id: "SEC-20", Title: "Control", Objective: "Objective",
					AssessmentRequirements: []gemara.AssessmentRequirement{{
						Id: "SEC-20.01", Text: "Retired", Applicability: tc.applicability,
						State: gemara.LifecycleRetired, ReplacedBy: &gemara.EntryMapping{EntryId: "SEC-20.02"},
					}},
				}},
			}
			suite := &EvaluationSuite{Name: "Only Retired", catalog: catalog, steps: map[string][]gemara.AssessmentStep{"other": {step_Pass}}}
			suite.config = setBasicConfig()

			if err := suite.Evaluate("testService"); err != nil {
				t.Fatalf("unexpected evaluate error: %v", err)
			}

			control := suite.EvaluationLog.Evaluations[0]
			if control.Result != gemara.NotRun {
				t.Errorf("control.Result = %v, want NotRun", control.Result)
			}
			// NotRun never changes gemara's rollup, so the control keeps no
			// message rather than gemara's "Not applicable: ..." text.
			if control.Message != "" {
				t.Errorf("control.Message = %q, want empty", control.Message)
			}
			if suite.Result != gemara.NotRun {
				t.Errorf("suite.Result = %v, want NotRun", suite.Result)
			}
			if suite.evalWarnings != 0 {
				t.Errorf("evalWarnings = %d, want 0", suite.evalWarnings)
			}
		})
	}

	t.Run("retired requirement that fails precheck stays Unknown with the precheck error", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "SEC-21", Title: "Control", Objective: "", // empty objective fails precheck
				AssessmentRequirements: []gemara.AssessmentRequirement{{
					Id: "SEC-21.01", Text: "Retired", Applicability: requestedApplicability,
					State: gemara.LifecycleRetired,
				}},
			}},
		}
		suite := &EvaluationSuite{Name: "Retired Precheck", catalog: catalog, steps: map[string][]gemara.AssessmentStep{"other": {step_Pass}}}
		suite.config = setBasicConfig()

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		log := suite.EvaluationLog.Evaluations[0].AssessmentLogs[0]
		if log.Result != gemara.Unknown {
			t.Errorf("Result = %v, want Unknown", log.Result)
		}
		if !strings.Contains(log.Message, "expected all AssessmentLog fields") {
			t.Errorf("Message = %q, want the precheck error", log.Message)
		}
	})

	t.Run("benchmark keeps retiredStep on retired requirements the plugin has steps for", func(t *testing.T) {
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "test-catalog"},
			Controls: []gemara.Control{{
				Id: "SEC-22", Title: "Control", Objective: "Objective",
				AssessmentRequirements: []gemara.AssessmentRequirement{
					{Id: "SEC-22.01", Text: "Active", Applicability: requestedApplicability},
					{Id: "SEC-22.02", Text: "Retired", Applicability: requestedApplicability, State: gemara.LifecycleRetired},
				},
			}},
		}
		suite := &EvaluationSuite{
			Name:    "Benchmark Retired",
			catalog: catalog,
			steps: map[string][]gemara.AssessmentStep{
				"SEC-22.01": {step_Pass},
				"SEC-22.02": {step_Pass},
			},
		}
		suite.config = setBasicConfig()
		suite.config.Benchmark = true

		if err := suite.Evaluate("testService"); err != nil {
			t.Fatalf("unexpected evaluate error: %v", err)
		}

		retiredLog := suite.EvaluationLog.Evaluations[0].AssessmentLogs[1]
		if len(retiredLog.Steps) != 1 || retiredLog.Steps[0].String() != "retired" {
			t.Errorf("retired steps = %v, want the single \"retired\" step", retiredLog.Steps)
		}
	})
}
