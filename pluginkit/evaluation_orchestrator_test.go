package pluginkit

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/goccy/go-yaml"
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

		err := orchestrator.AddReferenceCatalogs("catalog-test-data/valid", testDataFS)
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
		steps := map[string][]gemara.AssessmentStep{
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

func TestEvaluationOrchestrator_AddEvaluationSuite_DeduplicatesCatalogId(t *testing.T) {
	orchestrator := &EvaluationOrchestrator{}
	catalog := getTestCatalogWithRequirements()
	orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
		catalog.Metadata.Id: catalog,
	}

	steps := createPassingStepsMap()

	err := orchestrator.AddEvaluationSuite(catalog.Metadata.Id, nil, steps)
	if err != nil {
		t.Fatalf("First AddEvaluationSuite failed: %v", err)
	}

	// Second call with same catalog ID should be silently ignored
	err = orchestrator.AddEvaluationSuite(catalog.Metadata.Id, nil, steps)
	if err != nil {
		t.Fatalf("Second AddEvaluationSuite failed: %v", err)
	}

	if len(orchestrator.possibleSuites) != 1 {
		t.Errorf("Expected 1 suite (deduped), got %d", len(orchestrator.possibleSuites))
	}
}

func TestEvaluationOrchestrator_Mobilize_BreaksAfterMatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-mobilize-break-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfg := setBasicConfig()
	cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
	cfg.Write = true
	cfg.WriteDirectory = tmpDir
	cfg.Output = "yaml"

	catalog := getTestCatalogWithRequirements()
	steps := createPassingStepsMap()

	// Manually construct two suites with the same CatalogId to test break behavior
	orchestrator := &EvaluationOrchestrator{
		ServiceName: "test-service",
		PluginName:  "test-plugin",
		config:      cfg,
		possibleSuites: []*EvaluationSuite{
			{CatalogId: "CCC.ObjStor", catalog: catalog, steps: steps, config: cfg},
			{CatalogId: "CCC.ObjStor", catalog: catalog, steps: steps, config: cfg},
		},
	}

	err = orchestrator.Mobilize()
	if err != nil {
		t.Fatalf("Mobilize failed: %v", err)
	}

	// Should only execute the first matching suite due to break
	if len(orchestrator.Evaluation_Suites) != 1 {
		t.Errorf("Expected 1 suite (break after first match), got %d", len(orchestrator.Evaluation_Suites))
	}

	// Mobilize must leave each executed suite's EvaluationLog self-describing.
	log := orchestrator.Evaluation_Suites[0].EvaluationLog
	if log.Metadata.Type != gemara.EvaluationLogArtifact {
		t.Errorf("expected stamped artifact type after Mobilize, got %v", log.Metadata.Type)
	}
	if log.Target.Id == "" {
		t.Error("expected stamped target id after Mobilize, got empty")
	}
}

func TestEvaluationOrchestrator_AddEvaluationSuiteForAllCatalogs(t *testing.T) {
	t.Run("Error Without Reference Catalogs", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		steps := map[string][]gemara.AssessmentStep{
			"test-requirement": {step_Pass},
		}

		err := orchestrator.AddEvaluationSuiteForAllCatalogs(nil, steps)
		if err == nil {
			t.Error("Expected error when no reference catalogs are loaded")
		}
		if !strings.Contains(err.Error(), "no reference catalogs loaded") {
			t.Errorf("Expected 'no reference catalogs loaded' error, got: %v", err)
		}
	})

	t.Run("Error With Zero Controls Catalog", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		catalog := &gemara.ControlCatalog{
			Metadata: gemara.Metadata{Id: "empty-catalog"},
			Controls: []gemara.Control{},
		}
		orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
			catalog.Metadata.Id: catalog,
		}

		err := orchestrator.AddEvaluationSuiteForAllCatalogs(nil, map[string][]gemara.AssessmentStep{})
		if err == nil {
			t.Error("Expected error for catalog with zero controls")
		}
		if !strings.Contains(err.Error(), "no controls provided") {
			t.Errorf("Expected 'no controls provided' error, got: %v", err)
		}
	})

	t.Run("Registers Suite For Each Catalog", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		catalog1 := getTestCatalogWithID("CCC.ObjStor")
		catalog2 := getTestCatalogWithID("CCC.ObjStor-v2")
		orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
			catalog1.Metadata.Id: catalog1,
			catalog2.Metadata.Id: catalog2,
		}

		steps := map[string][]gemara.AssessmentStep{
			"CCC.Core.C01.TR01": {step_Pass},
		}

		err := orchestrator.AddEvaluationSuiteForAllCatalogs(nil, steps)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(orchestrator.possibleSuites) != 2 {
			t.Errorf("Expected 2 suites (one per catalog), got %d",
				len(orchestrator.possibleSuites))
		}

		// Verify each catalog got its own suite
		suiteIDs := make(map[string]bool)
		for _, suite := range orchestrator.possibleSuites {
			suiteIDs[suite.CatalogId] = true
		}
		if !suiteIDs["CCC.ObjStor"] || !suiteIDs["CCC.ObjStor-v2"] {
			t.Errorf("Expected suites for both catalogs, got: %v", suiteIDs)
		}
	})

	t.Run("Uses Provided Loader", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		catalog := getTestCatalogWithRequirements()
		orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
			catalog.Metadata.Id: catalog,
		}

		testLoader := func(cfg *config.Config) (interface{}, error) {
			return "suite-specific-payload", nil
		}
		steps := map[string][]gemara.AssessmentStep{
			"CCC.Core.C01.TR01": {step_Pass},
		}

		err := orchestrator.AddEvaluationSuiteForAllCatalogs(testLoader, steps)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(orchestrator.possibleSuites) != 1 {
			t.Errorf("Expected 1 suite for single catalog, got %d", len(orchestrator.possibleSuites))
		}

		for _, suite := range orchestrator.possibleSuites {
			if suite.loader == nil {
				t.Error("Expected suite loader to be set")
			}
		}
	})

	t.Run("Nil Steps", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		catalog := getTestCatalogWithRequirements()
		orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
			catalog.Metadata.Id: catalog,
		}

		err := orchestrator.AddEvaluationSuiteForAllCatalogs(nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(orchestrator.possibleSuites) != 1 {
			t.Errorf("Expected 1 suite, got %d", len(orchestrator.possibleSuites))
		}
		if orchestrator.possibleSuites[0].steps != nil {
			t.Error("Expected nil steps to be preserved")
		}
	})

	t.Run("Empty Steps", func(t *testing.T) {
		orchestrator := &EvaluationOrchestrator{}
		catalog := getTestCatalogWithRequirements()
		orchestrator.referenceCatalogs = map[string]*gemara.ControlCatalog{
			catalog.Metadata.Id: catalog,
		}

		emptySteps := map[string][]gemara.AssessmentStep{}
		err := orchestrator.AddEvaluationSuiteForAllCatalogs(nil, emptySteps)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(orchestrator.possibleSuites) != 1 {
			t.Errorf("Expected 1 suite, got %d", len(orchestrator.possibleSuites))
		}
		if len(orchestrator.possibleSuites[0].steps) != 0 {
			t.Error("Expected empty steps map to be preserved")
		}
	})
}

func TestEvaluationOrchestrator_Mobilize_UnmatchedCatalogs(t *testing.T) {
	t.Run("Error When No Requested Catalogs Match", func(t *testing.T) {
		cfg := setBasicConfig()
		cfg.Policy.ControlCatalogs = []string{"nonexistent-catalog"}

		orchestrator := &EvaluationOrchestrator{
			ServiceName: "test-service",
			PluginName:  "test-plugin",
			config:      cfg,
			possibleSuites: []*EvaluationSuite{
				{CatalogId: "catalog-a", config: cfg},
				{CatalogId: "catalog-b", config: cfg},
			},
		}

		err := orchestrator.Mobilize()
		if err == nil {
			t.Fatal("Expected error when no catalogs match")
		}
		if !strings.Contains(err.Error(), "no requested catalogs matched available suites") {
			t.Errorf("Expected 'no requested catalogs matched' error, got: %v", err)
		}
	})

	t.Run("Repeated Calls Clear Prior Results", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-mobilize-repeated-")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		cfg := setBasicConfig()
		cfg.Write = true
		cfg.WriteDirectory = tmpDir
		cfg.Output = "yaml"

		catalog := getTestCatalogWithRequirements()
		steps := createPassingStepsMap()

		orchestrator := &EvaluationOrchestrator{
			ServiceName: "test-service",
			PluginName:  "test-plugin",
			config:      cfg,
			possibleSuites: []*EvaluationSuite{
				{
					CatalogId: "CCC.ObjStor",
					catalog:   catalog,
					steps:     steps,
					config:    cfg,
				},
			},
		}

		// First call matches
		cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor"}
		err = orchestrator.Mobilize()
		if err != nil {
			t.Fatalf("First Mobilize() call failed: %v", err)
		}
		if len(orchestrator.Evaluation_Suites) != 1 {
			t.Fatalf("Expected 1 suite after first call, got %d", len(orchestrator.Evaluation_Suites))
		}

		// Second call with non-matching catalog should error, not silently succeed
		cfg.Policy.ControlCatalogs = []string{"nonexistent-catalog"}
		err = orchestrator.Mobilize()
		if err == nil {
			t.Fatal("Expected error on second Mobilize() with unmatched catalogs")
		}
		if !strings.Contains(err.Error(), "no requested catalogs matched available suites") {
			t.Errorf("Expected 'no requested catalogs matched' error, got: %v", err)
		}
	})

	t.Run("Partial Match Succeeds With Warning", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-mobilize-")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		cfg := setBasicConfig()
		cfg.Policy.ControlCatalogs = []string{"CCC.ObjStor", "nonexistent-catalog"}
		cfg.Write = true
		cfg.WriteDirectory = tmpDir
		cfg.Output = "yaml"

		catalog := getTestCatalogWithRequirements()
		steps := createPassingStepsMap()

		orchestrator := &EvaluationOrchestrator{
			ServiceName: "test-service",
			PluginName:  "test-plugin",
			config:      cfg,
			possibleSuites: []*EvaluationSuite{
				{
					CatalogId: "CCC.ObjStor",
					catalog:   catalog,
					steps:     steps,
					config:    cfg,
				},
			},
		}

		err = orchestrator.Mobilize()
		if err != nil {
			t.Errorf("Expected partial match to succeed, got error: %v", err)
		}
		if len(orchestrator.Evaluation_Suites) != 1 {
			t.Errorf("Expected 1 executed suite, got %d", len(orchestrator.Evaluation_Suites))
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

func createTestEvalLog() gemara.EvaluationLog {
	return gemara.EvaluationLog{
		Evaluations: []*gemara.ControlEvaluation{
			passingEvaluation(),
		},
		Metadata: gemara.Metadata{
			Author: gemara.Actor{
				Name:    "test-plugin",
				Uri:     "https://github.com/test/repo",
				Version: "1.0.0",
			},
		},
	}
}

func BenchmarkGetImportedControls(b *testing.B) {
	primary := &gemara.ControlCatalog{
		Metadata: gemara.Metadata{Id: "primary"},
		Controls: []gemara.Control{
			{Id: "P-1"},
		},
		Imports: []gemara.MultiEntryMapping{
			{
				ReferenceId: "imported",
				Entries: []gemara.ArtifactMapping{
					{ReferenceId: "I-1"},
					{ReferenceId: "I-2"},
				},
			},
		},
	}
	imported := &gemara.ControlCatalog{
		Metadata: gemara.Metadata{Id: "imported"},
		Controls: []gemara.Control{
			{Id: "I-1"},
			{Id: "I-2"},
			{Id: "I-3"},
		},
	}
	refs := map[string]*gemara.ControlCatalog{
		"primary":  primary,
		"imported": imported,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getImportedControls(primary, refs)
	}
}

func BenchmarkAddPossibleControls(b *testing.B) {
	catalog := getTestCatalogWithRequirements()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orchestrator := &EvaluationOrchestrator{}
		orchestrator.addPossibleControls(catalog)
	}
}

func BenchmarkGetPluginCatalogs(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := getPluginCatalogs("catalog-test-data/valid", testDataFS)
		if err != nil {
			b.Fatalf("getPluginCatalogs failed: %v", err)
		}
	}
}

func TestEvaluationOrchestrator_WriteResults_SARIF(t *testing.T) {
	tests := []struct {
		name      string
		pluginUri string
		catalog   *gemara.ControlCatalog
	}{
		{name: "with PluginUri, no catalog", pluginUri: "https://github.com/test/repo"},
		{name: "without PluginUri, no catalog", pluginUri: ""},
		{name: "with catalog enrichment", pluginUri: "https://github.com/test/repo", catalog: getTestCatalogWithRequirements()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
				PluginUri:     tc.pluginUri,
				PluginVersion: "1.0.0",
				config:        cfg,
			}

			suite := &EvaluationSuite{
				CatalogId:     "test-catalog",
				EvaluationLog: createTestEvalLog(),
				catalog:       tc.catalog,
				config:        cfg,
			}

			orchestrator.Evaluation_Suites = []*EvaluationSuite{suite}

			if err := orchestrator.WriteResults(); err != nil {
				t.Fatalf("WriteResults failed: %v", err)
			}

			outPath := filepath.Join(tmpDir, "test-service", "test-service.sarif")
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("expected SARIF output at %s: %v", outPath, err)
			}
			if len(data) == 0 {
				t.Fatal("SARIF output file is empty")
			}

			var report struct {
				Schema  string `json:"$schema"`
				Version string `json:"version"`
				Runs    []struct {
					Tool struct {
						Driver struct {
							Name           string `json:"name"`
							InformationURI string `json:"informationUri"`
							Version        string `json:"version"`
							Rules          []struct {
								ID string `json:"id"`
							} `json:"rules"`
						} `json:"driver"`
					} `json:"tool"`
				} `json:"runs"`
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatalf("SARIF output is not valid JSON: %v", err)
			}
			if report.Version != "2.1.0" {
				t.Errorf("expected SARIF version 2.1.0, got %q", report.Version)
			}
			if !strings.Contains(report.Schema, "sarif-schema-2.1.0") {
				t.Errorf("unexpected SARIF schema: %q", report.Schema)
			}
			if len(report.Runs) == 0 {
				t.Fatal("expected at least one SARIF run")
			}
			driver := report.Runs[0].Tool.Driver
			if driver.Name != "test-plugin" {
				t.Errorf("expected driver name %q, got %q", "test-plugin", driver.Name)
			}
			if driver.InformationURI != "https://github.com/test/repo" {
				t.Errorf("expected driver informationUri from EvaluationLog metadata, got %q", driver.InformationURI)
			}
		})
	}
}

// writeResultsFixture builds a minimal orchestrator with one suite, writes
// results to a temp directory, and returns the path to the produced file.
// It centralizes the boilerplate shared by the gemara and IncludePayload
// tests below.
func writeResultsFixture(t *testing.T, output string, includePayload bool, payload any) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "test-writeresults-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cfg := setBasicConfig()
	cfg.Output = output
	cfg.Write = true
	cfg.WriteDirectory = tmpDir
	cfg.IncludePayload = includePayload

	orchestrator := &EvaluationOrchestrator{
		ServiceName:   "test-service",
		PluginName:    "test-plugin",
		PluginUri:     "https://github.com/test/repo",
		PluginVersion: "1.0.0",
		Payload:       payload,
		config:        cfg,
	}
	orchestrator.Evaluation_Suites = []*EvaluationSuite{
		{
			CatalogId:     "test-catalog",
			EvaluationLog: createTestEvalLog(),
			config:        cfg,
		},
	}

	if err := orchestrator.WriteResults(); err != nil {
		t.Fatalf("WriteResults failed: %v", err)
	}

	// gemara output is written with a .yaml extension; everything else uses the
	// configured output name directly.
	ext := output
	if ext == "gemara" {
		ext = "yaml"
	}
	return filepath.Join(tmpDir, "test-service", "test-service."+ext)
}

func TestEvaluationOrchestrator_WriteResults_Gemara(t *testing.T) {
	t.Run("emits a list of EvaluationLog objects", func(t *testing.T) {
		resultPath := writeResultsFixture(t, "gemara", false, nil)

		data, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("expected gemara output file at %s: %v", resultPath, err)
		}

		// Always-list shape: downstream consumers can range over the result
		// without first checking whether they got a mapping or a sequence.
		// Unmarshal into a generic list rather than []gemara.EvaluationLog so
		// the shape assertion does not depend on gemara's strict field
		// validation (which rejects unset enum defaults in test fixtures).
		var logs []map[string]any
		if err := yaml.Unmarshal(data, &logs); err != nil {
			t.Fatalf("gemara output is not a list: %v\noutput was:\n%s", err, data)
		}
		if len(logs) != 1 {
			t.Errorf("expected 1 EvaluationLog (one suite), got %d", len(logs))
		}
		// Verify it really is the gemara shape (has metadata + evaluations
		// at the top level of each entry) rather than the orchestrator envelope.
		if _, ok := logs[0]["metadata"]; !ok {
			t.Errorf("expected gemara EvaluationLog shape with 'metadata' key, got: %v", logs[0])
		}
		if _, ok := logs[0]["evaluations"]; !ok {
			t.Errorf("expected gemara EvaluationLog shape with 'evaluations' key, got: %v", logs[0])
		}

		// The gemara branch must not wrap output in the orchestrator envelope.
		if strings.Contains(string(data), "service-name") || strings.Contains(string(data), "plugin-name") {
			t.Errorf("gemara output should not contain orchestrator envelope fields, got:\n%s", data)
		}
	})

	t.Run("empty suite list still emits a list, not null", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-writeresults-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		cfg := setBasicConfig()
		cfg.Output = "gemara"
		cfg.Write = true
		cfg.WriteDirectory = tmpDir

		orchestrator := &EvaluationOrchestrator{
			ServiceName: "test-service",
			PluginName:  "test-plugin",
			config:      cfg,
		}
		if err := orchestrator.WriteResults(); err != nil {
			t.Fatalf("WriteResults failed: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(tmpDir, "test-service", "test-service.yaml"))
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		// Expect "[]" — a valid empty sequence — rather than null, so consumers
		// don't have to special-case the no-suites condition.
		if strings.TrimSpace(string(data)) != "[]" {
			t.Errorf("expected empty list output, got: %q", strings.TrimSpace(string(data)))
		}
	})
}

func TestEvaluationOrchestrator_StampEvaluationLog(t *testing.T) {
	newOrchestrator := func() *EvaluationOrchestrator {
		return &EvaluationOrchestrator{
			ServiceName:   "test-service",
			PluginName:    "test-plugin",
			PluginUri:     "https://github.com/test/repo",
			PluginVersion: "1.0.0",
			config:        setBasicConfig(),
		}
	}

	t.Run("populates self-describing metadata, result, and default target", func(t *testing.T) {
		orchestrator := newOrchestrator()
		suite := &EvaluationSuite{CatalogId: "test-catalog", Result: gemara.Passed}

		orchestrator.stampEvaluationLog(suite)

		m := suite.EvaluationLog.Metadata
		if m.Id != "test-service_test-catalog" {
			t.Errorf("expected stable metadata id, got %q", m.Id)
		}
		if m.Type != gemara.EvaluationLogArtifact {
			t.Errorf("expected artifact type EvaluationLog, got %v", m.Type)
		}
		if m.GemaraVersion != gemara.SchemaVersion {
			t.Errorf("expected gemara-version %q, got %q", gemara.SchemaVersion, m.GemaraVersion)
		}
		if _, err := time.Parse(time.RFC3339, string(m.Date)); err != nil {
			t.Errorf("expected RFC3339 metadata date, got %q: %v", m.Date, err)
		}
		if m.Description == "" || strings.Contains(m.Description, "corrupted-state") {
			t.Errorf("expected description without corruption marker, got %q", m.Description)
		}
		if m.Author.Name != "test-plugin" || m.Author.Uri != "https://github.com/test/repo" || m.Author.Version != "1.0.0" {
			t.Errorf("expected plugin provenance in author, got %+v", m.Author)
		}
		if m.Author.Id != "test-plugin" || m.Author.Type != gemara.Software {
			t.Errorf("expected valid author id and type, got %+v", m.Author)
		}
		if suite.EvaluationLog.Result != gemara.Passed {
			t.Errorf("expected log result to mirror suite result, got %v", suite.EvaluationLog.Result)
		}
		target := suite.EvaluationLog.Target
		if target.Id != "test-service" || target.Name != "test-service" || target.Type != gemara.Software {
			t.Errorf("expected default target from service name, got %+v", target)
		}
	})

	t.Run("author id uses the publish coordinate when Publisher is set", func(t *testing.T) {
		orchestrator := newOrchestrator()
		orchestrator.Publisher = "test-org"
		suite := &EvaluationSuite{CatalogId: "test-catalog"}

		orchestrator.stampEvaluationLog(suite)

		if got := suite.EvaluationLog.Metadata.Author.Id; got != "test-org/test-plugin" {
			t.Errorf("expected publish coordinate as author id, got %q", got)
		}
	})

	t.Run("marks corrupted state in the description", func(t *testing.T) {
		orchestrator := newOrchestrator()
		suite := &EvaluationSuite{CatalogId: "test-catalog", CorruptedState: true}

		orchestrator.stampEvaluationLog(suite)

		if !strings.Contains(suite.EvaluationLog.Metadata.Description, "corrupted-state: true") {
			t.Errorf("expected corruption marker in description, got %q", suite.EvaluationLog.Metadata.Description)
		}
	})

	t.Run("target builder is used with empty fields backfilled", func(t *testing.T) {
		orchestrator := newOrchestrator()
		orchestrator.AddTargetBuilder(func(cfg *config.Config) gemara.Resource {
			return gemara.Resource{
				Id:  "github.com/test-owner/test-repo",
				Uri: "https://github.com/test-owner/test-repo",
			}
		})
		suite := &EvaluationSuite{CatalogId: "test-catalog"}

		orchestrator.stampEvaluationLog(suite)

		target := suite.EvaluationLog.Target
		if target.Id != "github.com/test-owner/test-repo" {
			t.Errorf("expected builder-provided target id, got %q", target.Id)
		}
		if target.Uri != "https://github.com/test-owner/test-repo" {
			t.Errorf("expected builder-provided target uri, got %q", target.Uri)
		}
		if target.Name != "test-service" {
			t.Errorf("expected empty target name backfilled from service name, got %q", target.Name)
		}
		if target.Type != gemara.Software {
			t.Errorf("expected empty target type backfilled, got %v", target.Type)
		}
	})
}

func TestEvaluationOrchestrator_WriteResults_IncludePayload(t *testing.T) {
	type orchestratorOutput struct {
		Payload any `yaml:"payload"`
	}

	largePayload := map[string]any{"trace": "details", "size": "large"}

	t.Run("payload is omitted by default", func(t *testing.T) {
		resultPath := writeResultsFixture(t, "yaml", false, largePayload)

		data, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if strings.Contains(string(data), "payload:") {
			t.Errorf("payload should be omitted by default, but found 'payload:' in output:\n%s", data)
		}
	})

	t.Run("payload is included when IncludePayload is true", func(t *testing.T) {
		resultPath := writeResultsFixture(t, "yaml", true, largePayload)

		data, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		var out orchestratorOutput
		if err := yaml.Unmarshal(data, &out); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if out.Payload == nil {
			t.Errorf("payload should be present when IncludePayload=true, got nil in output:\n%s", data)
		}
	})

	t.Run("orchestrator Payload is restored after WriteResults", func(t *testing.T) {
		// Verifies the snapshot-and-restore: calling WriteResults must not
		// permanently mutate v.Payload, so repeated calls and post-write
		// inspection behave consistently.
		tmpDir, err := os.MkdirTemp("", "test-writeresults-")
		if err != nil {
			t.Fatalf("temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		cfg := setBasicConfig()
		cfg.Output = "yaml"
		cfg.Write = true
		cfg.WriteDirectory = tmpDir
		cfg.IncludePayload = false

		orchestrator := &EvaluationOrchestrator{
			ServiceName: "test-service",
			PluginName:  "test-plugin",
			Payload:     largePayload,
			config:      cfg,
		}
		orchestrator.Evaluation_Suites = []*EvaluationSuite{
			{CatalogId: "test-catalog", EvaluationLog: createTestEvalLog(), config: cfg},
		}

		if err := orchestrator.WriteResults(); err != nil {
			t.Fatalf("WriteResults failed: %v", err)
		}
		if orchestrator.Payload == nil {
			t.Errorf("WriteResults must not permanently null out Payload; second-call inspections would be wrong")
		}
	})
}
