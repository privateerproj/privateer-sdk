package pluginkit

import (
	"maps"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/shared"
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

// TestEvaluate_MalformedRequirements pins how a catalog entry that cannot be
// judged rolls up. The SDK decides these results itself, so they must not
// change when gemara changes how its own precheck feeds the control result.
func TestEvaluate_MalformedRequirements(t *testing.T) {
	const objective = "Ensure the thing is secure."
	outOfScope := []string{"tlp-red"}

	control := func(id, objective string, reqs ...gemara.AssessmentRequirement) gemara.Control {
		return gemara.Control{Id: id, Title: id, Objective: objective, AssessmentRequirements: reqs}
	}
	requirement := func(id string, applicability []string) gemara.AssessmentRequirement {
		return gemara.AssessmentRequirement{Id: id, Text: id, Applicability: applicability}
	}

	type wantAssessment struct {
		result      gemara.Result
		msgContains []string
	}

	tests := []struct {
		name              string
		controls          []gemara.Control
		steps             map[string][]gemara.AssessmentStep
		userApplicability []string // nil keeps requestedApplicability
		wantControls      map[string]gemara.Result
		wantAssessments   map[string]wantAssessment
		wantSuite         gemara.Result
		wantExit          int
	}{
		{
			name:     "empty applicability fails the control",
			controls: []gemara.Control{control("C1", objective, requirement("C1.R1", nil))},
			steps:    map[string][]gemara.AssessmentStep{"C1.R1": {step_Pass}},
			wantControls: map[string]gemara.Result{
				"C1": gemara.Failed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.Failed, []string{`"C1.R1"`, `"C1"`, "applicability"}},
			},
			wantSuite: gemara.Failed,
			wantExit:  shared.TestFail,
		},
		{
			name: "empty applicability fails the control despite a passing sibling",
			controls: []gemara.Control{
				control("C1", objective,
					requirement("C1.R1", requestedApplicability),
					requirement("C1.R2", []string{}),
				),
				control("C2", objective, requirement("C2.R1", requestedApplicability)),
			},
			steps: map[string][]gemara.AssessmentStep{
				"C1.R1": {step_Pass},
				"C1.R2": {step_Pass},
				"C2.R1": {step_Pass},
			},
			wantControls: map[string]gemara.Result{
				"C1": gemara.Failed,
				"C2": gemara.Passed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.Passed, nil},
				"C1.R2": {gemara.Failed, []string{`"C1.R2"`, "applicability"}},
				"C2.R1": {gemara.Passed, nil},
			},
			wantSuite: gemara.Failed,
			wantExit:  shared.TestFail,
		},
		{
			name: "empty applicability fails even when the user filter excludes its siblings",
			controls: []gemara.Control{
				control("C1", objective,
					requirement("C1.R1", requestedApplicability),
					requirement("C1.R2", nil),
				),
			},
			steps: map[string][]gemara.AssessmentStep{
				"C1.R1": {step_Pass},
				"C1.R2": {step_Pass},
			},
			userApplicability: outOfScope,
			wantControls: map[string]gemara.Result{
				"C1": gemara.Failed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.NotApplicable, nil},
				"C1.R2": {gemara.Failed, []string{`"C1.R2"`, "applicability"}},
			},
			wantSuite: gemara.Failed,
			wantExit:  shared.TestFail,
		},
		{
			name:     "empty objective fails the control",
			controls: []gemara.Control{control("C1", "", requirement("C1.R1", requestedApplicability))},
			steps:    map[string][]gemara.AssessmentStep{"C1.R1": {step_Pass}},
			wantControls: map[string]gemara.Result{
				"C1": gemara.Failed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.Failed, []string{`"C1.R1"`, `"C1"`, "objective"}},
			},
			wantSuite: gemara.Failed,
			wantExit:  shared.TestFail,
		},
		{
			name:     "empty objective fails the control outside the user's applicability",
			controls: []gemara.Control{control("C1", "", requirement("C1.R1", outOfScope))},
			steps:    map[string][]gemara.AssessmentStep{"C1.R1": {step_Pass}},
			wantControls: map[string]gemara.Result{
				"C1": gemara.Failed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.Failed, []string{`"C1"`, "objective"}},
			},
			wantSuite: gemara.Failed,
			wantExit:  shared.TestFail,
		},
		{
			name: "applicable requirement with no steps is Unknown",
			controls: []gemara.Control{
				control("C1", objective, requirement("C1.R1", requestedApplicability)),
				control("C2", objective, requirement("C2.R1", requestedApplicability)),
			},
			steps: map[string][]gemara.AssessmentStep{"C2.R1": {step_Pass}},
			wantControls: map[string]gemara.Result{
				"C1": gemara.Unknown,
				"C2": gemara.Passed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.Unknown, nil},
				"C2.R1": {gemara.Passed, nil},
			},
			wantSuite: gemara.Unknown,
			wantExit:  shared.TestFail,
		},
		{
			name: "requirement with no steps outside the user's applicability is NotApplicable",
			controls: []gemara.Control{
				control("C1", objective, requirement("C1.R1", outOfScope)),
				control("C2", objective, requirement("C2.R1", requestedApplicability)),
			},
			steps: map[string][]gemara.AssessmentStep{"C2.R1": {step_Pass}},
			wantControls: map[string]gemara.Result{
				"C1": gemara.NotApplicable,
				"C2": gemara.Passed,
			},
			wantAssessments: map[string]wantAssessment{
				"C1.R1": {gemara.NotApplicable, nil},
				"C2.R1": {gemara.Passed, nil},
			},
			wantSuite: gemara.Passed,
			wantExit:  shared.TestPass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite := &EvaluationSuite{
				CatalogId: "test-catalog",
				catalog: &gemara.ControlCatalog{
					Metadata: gemara.Metadata{Id: "test-catalog"},
					Controls: tt.controls,
				},
				steps:  tt.steps,
				config: setBasicConfig(),
			}
			if tt.userApplicability != nil {
				suite.config.Policy.Applicability = tt.userApplicability
			}

			if err := suite.Evaluate("testService"); err != nil {
				t.Fatalf("Evaluate: unexpected error: %v", err)
			}

			gotControls := make(map[string]gemara.Result, len(suite.EvaluationLog.Evaluations))
			for _, evaluation := range suite.EvaluationLog.Evaluations {
				gotControls[evaluation.Control.EntryId] = evaluation.Result
				for _, assessment := range evaluation.AssessmentLogs {
					id := assessment.Requirement.EntryId
					want, ok := tt.wantAssessments[id]
					if !ok {
						t.Errorf("unexpected assessment %q", id)
						continue
					}
					if assessment.Result != want.result {
						t.Errorf("assessment %q result: got = %v, want = %v (message %q)", id, assessment.Result, want.result, assessment.Message)
					}
					for _, sub := range want.msgContains {
						if !strings.Contains(assessment.Message, sub) {
							t.Errorf("assessment %q message: got = %q, want it to contain %q", id, assessment.Message, sub)
						}
					}
					if want.result == gemara.Failed && evaluation.Result == gemara.Failed && evaluation.Message != assessment.Message {
						t.Errorf("control %q message: got = %q, want = %q", evaluation.Control.EntryId, evaluation.Message, assessment.Message)
					}
				}
			}
			if !maps.Equal(gotControls, tt.wantControls) {
				t.Errorf("control results: got = %v, want = %v", gotControls, tt.wantControls)
			}
			if suite.Result != tt.wantSuite {
				t.Errorf("suite result: got = %v, want = %v", suite.Result, tt.wantSuite)
			}
			orch := &EvaluationOrchestrator{Evaluation_Suites: []*EvaluationSuite{suite}}
			if got := ExitCodeFor(orch, nil); got != tt.wantExit {
				t.Errorf("ExitCodeFor: got = %d, want = %d", got, tt.wantExit)
			}
		})
	}
}
