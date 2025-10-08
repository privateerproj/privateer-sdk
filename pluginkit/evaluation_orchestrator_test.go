package pluginkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ossf/gemara/layer2"
	"github.com/ossf/gemara/layer4"
	"github.com/privateerproj/privateer-sdk/config"
)

func TestSetPayload(t *testing.T) {
	v := &EvaluationOrchestrator{
		PluginName: "test",
	}

	payloadTestData := []struct {
		name     string
		payload  interface{}
		expected bool
	}{
		{
			name:    "nil payload",
			payload: nil,
		},
		{
			name: "string payload",
			payload: PayloadTypeExample{
				CustomPayloadField: true,
			},
		},
	}

	for _, test := range payloadTestData {
		t.Run(test.name, func(t *testing.T) {
			v.loader = func(*config.Config) (interface{}, error) { return &test.payload, nil }
			err := v.loadPayload()
			if err != nil {
				t.Errorf("Expected no error, but got %v", err)
				return
			}
			if v.Payload == nil {
				t.Error("expected v.Payload to never be nil")
			}
			// TODO: Add a test to check if the loaded payload is the same as the test payload
		})
	}
}

func TestSetupConfig(t *testing.T) {
	v := &EvaluationOrchestrator{
		PluginName: "test",
	}
	v.setupConfig()
	if v.config == nil {
		t.Error("Expected config to always be returned")
	}
}

func TestAddEvaluationSuite(t *testing.T) {
	testData := []testingData{
		{
			testName:       "Good Evaluation",
			expectedResult: layer4.Passed,
			steps:          createPassingStepsMap(),
		},
	}
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			catalog, err := getTestCatalog()
			if err != nil {
				t.Errorf("Expected no error loading test catalog, but got %v", err)
				return
			}
			v := &EvaluationOrchestrator{
				PluginName: "test",
			}
			v.config = setBasicConfig()
			v.AddEvaluationSuite(examplePayload, test.steps, catalog)
			if len(v.possibleSuites) == 0 {
				t.Error("Expected evaluation suites to be set")
				return
			}
			for _, suite := range v.possibleSuites {
				if suite.Name != "" {
					t.Errorf("Expected pending evaluation suite name to be unset, but got %s", suite.Name)
				}
				if suite.CatalogId != catalog.Metadata.Id {
					t.Errorf("Expected catalog ID to match, but got %s instead of %s", suite.CatalogId, catalog.Metadata.Id)
				}
				if len(suite.steps) != len(test.steps) {
					t.Errorf("Expected steps to match test data, but got %v", suite.steps)
				}
				if suite.config != v.config {
					t.Errorf("Expected config to match simpleConfig but got %v", suite.config)
				}
				if suite.loader == nil {
					t.Error("Expected loader to be set")
				}
				if suite.catalog != catalog {
					t.Error("Expected catalog to be set correctly")
				}
			}
		})
	}
}

func TestMobilize(t *testing.T) {
	for _, test := range mobilizeTestData {
		t.Run(test.testName, func(tt *testing.T) {
			var limitedConfigEvaluationCount int

			tt.Run("limitedConfig", func(tt *testing.T) {
				v := &EvaluationOrchestrator{
					PluginName: "test",
				}
				v.config = setLimitedConfig()

				catalog := &layer2.Catalog{
					Metadata: layer2.Metadata{
						Id: strings.ReplaceAll(test.testName, " ", "-"),
					},
				}
				v.AddEvaluationSuite(examplePayload, convertEvalsToStepsMap(test.evals), catalog)

				// grab a count of the applicable evaluations when config is limited
				err := v.Mobilize()
				if err != nil {
					tt.Errorf("Expected no error, but got %v", err)
				}
				limitedConfigEvaluationCount = len(v.Evaluation_Suites)
			})

			// tt.Run("non-invasive", func(tt *testing.T) {
			// 	runMobilizeTests(tt, test, false, limitedConfigEvaluationCount)
			// })
			tt.Run("invasive", func(tt *testing.T) {
				runMobilizeTests(tt, test, true, limitedConfigEvaluationCount)
			})
		})
	}
}

func runMobilizeTests(t *testing.T, test testingData, invasive bool, limitedConfigEvaluationCount int) {
	catalog, err := getTestCatalog()
	if err != nil {
		t.Errorf("Expected no error loading test catalog, but got %v", err)
		return
	}

	catalogName := strings.ReplaceAll(test.testName, " ", "-")
	catalog.Metadata.Id = catalogName
	v := &EvaluationOrchestrator{
		PluginName: "test",
	}
	v.config = setBasicConfig()
	v.config.Invasive = invasive

	v.AddEvaluationSuite(examplePayload, convertEvalsToStepsMap(test.evals), catalog)

	// Nothing from our test data should be applicable right now, but they should be possible
	err = v.Mobilize()
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if len(v.possibleSuites) == 0 {
		t.Errorf("Expected evaluation suites to be set, but got %v", v.possibleSuites)
		return
	}
	if len(v.Evaluation_Suites) > 0 {
		t.Errorf("Expected no Evaluation Suites to be set, but got %v", len(v.possibleSuites))
		return
	}

	// Now we set the catalog to be applicable, then run Mobilize again to find results
	v.config.Policy.ControlCatalogs = []string{catalogName}
	_ = v.Mobilize()
	if len(v.Evaluation_Suites) == 0 {
		t.Errorf("Expected evaluation suites to be set, but got %v", v.Evaluation_Suites)
		return
	}
	if len(v.Evaluation_Suites) == limitedConfigEvaluationCount {
		t.Errorf("Expected fewer Evaluation Suites to be when using limited config, but got the same count")
		return
	}

	for _, suite := range v.Evaluation_Suites {
		t.Run(suite.Name, func(tt *testing.T) {
			if len(test.evals) != len(suite.EvaluationLog.Evaluations) {
				tt.Errorf("Expected %v control evaluations, but got %v", len(test.evals), len(v.Evaluation_Suites))
			}
			if test.expectedResult != suite.Result {
				tt.Errorf("Expected result to be %v, but got %v", test.expectedResult, suite.Result)
			}
			if v.config.Invasive && suite.CorruptedState != test.expectedCorruption {
				tt.Errorf("Expected corrupted state to be %v, but got %v", test.expectedCorruption, suite.CorruptedState)
			}
			evaluationSuiteName := fmt.Sprintf("%s_%s", v.ServiceName, catalogName)
			if suite.Name != evaluationSuiteName {
				tt.Errorf("Expected evaluation suite name to be %s, but got %s", evaluationSuiteName, suite.Name)
			}
			for _, evaluatedSuite := range v.Evaluation_Suites {
				if len(suite.EvaluationLog.Evaluations) != len(evaluatedSuite.EvaluationLog.Evaluations) {
					tt.Errorf("Expected control evaluations to match test data, but got %v", evaluatedSuite.EvaluationLog.Evaluations)
				}
				testPayloadData := testPayload.(PayloadTypeExample)
				suitePayloadData := evaluatedSuite.payload.(PayloadTypeExample)
				if testPayloadData != suitePayloadData {
					tt.Errorf("Expected payload to be %v, but got %v", testPayloadData, suitePayloadData)
				}
				if evaluatedSuite.config != v.config {
					tt.Errorf("Expected config to match simpleConfig but got %v", evaluatedSuite.config)
				}
			}
		})
	}
}
