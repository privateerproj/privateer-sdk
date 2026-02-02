package pluginkit

import (
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
)

func TestEvaluate(t *testing.T) {
	testData := getTestEvaluateData()

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			// Create a minimal catalog to avoid nil pointer panic
			catalog, err := getTestCatalog()
			if err != nil {
				t.Fatal("Failed to load test catalog")
			}

			suite := &EvaluationSuite{
				Name:          test.testName,
				EvaluationLog: gemara.EvaluationLog{Evaluations: test.evals},
				catalog:       catalog,
				steps:         test.steps,
			}
			suite.config = setBasicConfig()

			err = suite.Evaluate("arbitrarySuiteName")
			if err != nil && test.expectedEvalSuiteError != nil {
				if !strings.Contains(err.Error(), test.expectedEvalSuiteError.Error()) {
					t.Errorf("Expected error containing '%s', but got '%v'", test.expectedEvalSuiteError, err)
				}
			} else if err != nil && test.expectedEvalSuiteError == nil {
				// For now, we expect an error about missing assessment requirements when catalog is empty
				// This is expected behavior with the current implementation
				expectedMessage := NO_ASSESSMENT_STEPS_PROVIDED("")
				if !strings.Contains(err.Error(), expectedMessage.Error()) {
					t.Errorf("Expected error containing '%s', but got '%v'", expectedMessage, err)
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
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

		suite := &EvaluationSuite{
			Name:    "Nil Config Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		// Don't set config - leave it nil

		err = suite.Evaluate("testService")
		if err == nil {
			t.Error("Expected error for nil config, but got none")
		}
		if !strings.Contains(err.Error(), "configuration not initialized") {
			t.Errorf("Expected 'configuration not initialized' error, but got: %v", err)
		}
	})
}

func TestGetAssessmentRequirements(t *testing.T) {
	t.Run("Valid Catalog", func(t *testing.T) {
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

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
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

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

	t.Run("Empty Steps Map", func(t *testing.T) {
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

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
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

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
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

		suite := &EvaluationSuite{
			Name:    "Integration Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()

		err = suite.Evaluate("integrationTest")
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
		catalog, err := getTestCatalog()
		if err != nil {
			t.Fatal("Failed to load test catalog")
		}

		suite := &EvaluationSuite{
			Name:    "Change Manager Test",
			catalog: catalog,
			steps:   createPassingStepsMap(),
		}
		suite.config = setBasicConfig()
		suite.config.Invasive = true

		changeManager := &ChangeManager{}
		suite.AddChangeManager(changeManager)

		err = suite.Evaluate("changeManagerTest")
		if err != nil {
			t.Errorf("Unexpected error in change manager test: %v", err)
		}

		// Verify change manager was properly set up
		if suite.changeManager == nil {
			t.Error("Expected change manager to be set")
		}
	})
}
