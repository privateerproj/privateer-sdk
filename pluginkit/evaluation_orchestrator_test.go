package pluginkit

import (
	"embed"
	"os"
	"strings"
	"testing"

	"github.com/ossf/gemara/layer4"
	"github.com/privateerproj/privateer-sdk/config"
)

func TestEvaluationOrchestrator_AddLoader(t *testing.T) {
	orchestrator := &EvaluationOrchestrator{}

	testLoader := func(cfg *config.Config) (interface{}, error) {
		return "test-payload", nil
	}

	orchestrator.AddLoader(testLoader)

	if orchestrator.loader == nil {
		t.Error("Expected loader to be set")
	}

	// Test that the loader works
	result, err := orchestrator.loader(nil)
	if err != nil {
		t.Errorf("Unexpected error from loader: %v", err)
	}
	if result != "test-payload" {
		t.Errorf("Expected 'test-payload', got %v", result)
	}
}

func TestEvaluationOrchestrator_AddRequiredVars(t *testing.T) {
	orchestrator := &EvaluationOrchestrator{}

	vars := []string{"VAR1", "VAR2", "VAR3"}
	orchestrator.AddRequiredVars(vars)

	if len(orchestrator.requiredVars) != len(vars) {
		t.Errorf("Expected %d vars, got %d", len(vars), len(orchestrator.requiredVars))
	}

	for i, expectedVar := range vars {
		if orchestrator.requiredVars[i] != expectedVar {
			t.Errorf("Expected var %d to be %s, got %s", i, expectedVar, orchestrator.requiredVars[i])
		}
	}
}

func TestEvaluationOrchestrator_AddReferenceCatalogs(t *testing.T) {
	t.Run("Empty Directory Name", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}

		err := orchestrator.AddReferenceCatalogs("", testDataFS)
		if err == nil {
			t.Error("Expected error for empty directory name")
		}

		if !strings.Contains(err.Error(), "data directory name cannot be empty") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Non-existent Directory", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		var emptyFS embed.FS

		err := orchestrator.AddReferenceCatalogs("non-existent", emptyFS)
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}

		if !strings.Contains(err.Error(), "no contents found in directory") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Nil Reference Catalogs Map", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		orchestrator.referenceCatalogs = nil

		err := orchestrator.AddReferenceCatalogs("catalog-test-data", testDataFS)
		if err != nil {
			// This might fail due to catalog validation, which is expected
			t.Logf("Expected error due to catalog validation: %v", err)
		}

		if orchestrator.referenceCatalogs == nil {
			t.Error("Expected referenceCatalogs to be initialized")
		}
	})
}

func TestEvaluationOrchestrator_AddEvaluationSuite(t *testing.T) {
	t.Run("Error Without Reference Catalogs", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}

		testLoader := func(cfg *config.Config) (interface{}, error) {
			return "test-payload", nil
		}
		steps := map[string][]layer4.AssessmentStep{
			"test-requirement": {step_Pass},
		}

		err := orchestrator.AddEvaluationSuite("test-catalog", testLoader, steps)
		if err == nil {
			t.Error("Expected error when no reference catalogs are set")
		}

		if len(orchestrator.possibleSuites) != 0 {
			t.Errorf("Expected 0 suites, got %d", len(orchestrator.possibleSuites))
		}
	})
}

func TestEvaluationOrchestrator_Integration(t *testing.T) {
	t.Run("Basic Setup", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{
			ServiceName:   "test-service",
			PluginName:    "test-plugin",
			PluginUri:     "test-uri",
			PluginVersion: "1.0.0",
		}

		// Add loader
		testLoader := func(cfg *config.Config) (interface{}, error) {
			return "test-payload", nil
		}
		orchestrator.AddLoader(testLoader)

		// Add required vars
		vars := []string{"TEST_VAR"}
		orchestrator.AddRequiredVars(vars)

		// Verify basic components are set
		if orchestrator.loader == nil {
			t.Error("Expected loader to be set")
		}
		if len(orchestrator.requiredVars) != 1 {
			t.Error("Expected 1 required var")
		}
		if orchestrator.ServiceName != "test-service" {
			t.Error("Expected service name to be set")
		}
		if orchestrator.PluginName != "test-plugin" {
			t.Error("Expected plugin name to be set")
		}
	})
}

func createTestEvalLog() layer4.EvaluationLog {
	return layer4.EvaluationLog{
		Evaluations: []*layer4.ControlEvaluation{
			passingEvaluation(),
		},
		Metadata: layer4.Metadata{
			Author: layer4.Author{
				Name:    "test-plugin",
				Uri:     "https://github.com/test/repo",
				Version: "1.0.0",
			},
		},
	}
}

func TestEvaluationOrchestrator_WriteResults_SARIF(t *testing.T) {
	t.Run("SARIF output with PluginUri", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-sarif-")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		cfg := setBasicConfig()
		cfg.Output = "sarif"
		cfg.Write = true
		cfg.WriteDirectory = tmpDir

		orchestrator := &EvaluationOrchestrator{
			ServiceName:   "test-service",
			PluginName:    "test-plugin",
			PluginUri:     "https://github.com/test/repo",
			PluginVersion: "1.0.0",
			config:        cfg,
		}

		suite := &EvaluationSuite{
			CatalogId:     "test-catalog",
			EvaluationLog: createTestEvalLog(),
			config:        cfg,
		}

		orchestrator.Evaluation_Suites = []*EvaluationSuite{suite}

		err = orchestrator.WriteResults()
		if err != nil {
			t.Errorf("WriteResults failed: %v", err)
		}
	})

	t.Run("SARIF output without PluginUri", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-sarif-")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		cfg := setBasicConfig()
		cfg.Output = "sarif"
		cfg.Write = true
		cfg.WriteDirectory = tmpDir

		orchestrator := &EvaluationOrchestrator{
			ServiceName:   "test-service",
			PluginName:    "test-plugin",
			PluginUri:     "",
			PluginVersion: "1.0.0",
			config:        cfg,
		}

		suite := &EvaluationSuite{
			CatalogId:     "test-catalog",
			EvaluationLog: createTestEvalLog(),
			config:        cfg,
		}

		orchestrator.Evaluation_Suites = []*EvaluationSuite{suite}

		err = orchestrator.WriteResults()
		if err != nil {
			t.Errorf("WriteResults failed: %v", err)
		}
	})
}
