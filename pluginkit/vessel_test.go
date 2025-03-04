package pluginkit

// This file contains table tests for the following functions:
// func (v *Vessel) SetInitilizer(initializer func(*config.Config) error) {
// func (v *Vessel) SetPayload(payload interface{}) {
// func (v *Vessel) Config() *config.Config {
// func (v *Vessel) AddEvaluationSuite(name string, evaluations []layer4.ControlEvaluation) {
// func (v *Vessel) Mobilize(requiredVars []string, suites map[string]EvaluationSuite) error {

import (
	"fmt"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
)

func TestSetPayload(t *testing.T) {
	v := NewVessel("test", nil, []string{})

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

func TestConfig(t *testing.T) {
	v := NewVessel("test", nil, []string{})
	v.setupConfig()
	if v.config == nil {
		t.Error("Expected config to be returned")
	}
}

func TestAddEvaluationSuite(t *testing.T) {
	testData := []testingData{
		{
			testName:       "Good Evaluation",
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
		},
	}
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			for _, suite := range test.evals {
				t.Run("subtest_"+suite.Name, func(t *testing.T) {
					v := NewVessel("test", nil, []string{})
					v.config = setSimpleConfig()
					v.AddEvaluationSuite("test", nil, test.evals)
					if v.possibleSuites == nil || len(v.possibleSuites) == 0 {
						t.Error("Expected evaluation suites to be set")
						return
					}
					for _, suite := range v.possibleSuites {
						if suite.Name != "" {
							t.Errorf("Expected pending evaluation suite name to be unset, but got %s", suite.Name)
						}
						if len(suite.Control_Evaluations) != len(test.evals) {
							t.Errorf("Expected control evaluations to match test data, but got %v", suite.Control_Evaluations)
						}
						if suite.config != v.config {
							t.Errorf("Expected config to match simpleConfig but got %v", suite.config)
						}
					}
				})
			}
		})
	}
}

func TestMobilize(t *testing.T) {
	testData := []testingData{
		{
			testName:       "Pass Evaluation",
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
		},
		{
			testName:       "Fail Evaluation",
			expectedResult: layer4.Failed,
			evals: []*layer4.ControlEvaluation{
				failingEvaluation(),
			},
		},
		{
			testName:       "Needs Review Evaluation",
			expectedResult: layer4.NeedsReview,
			evals: []*layer4.ControlEvaluation{
				needsReviewEvaluation(),
			},
		},
		{
			testName:           "Corrupted Evaluation",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
			},
		},
		{
			testName:       "Pass, Pass, Pass",
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				passingEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:       "Pass Then Fail",
			expectedResult: layer4.Failed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				failingEvaluation(),
			},
		},
		{
			testName:       "Pass Then Needs Review",
			expectedResult: layer4.NeedsReview,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				needsReviewEvaluation(),
			},
		},
		{
			testName:           "Pass Then Corrupted",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				corruptedEvaluation(),
			},
		},
		{
			testName:       "Needs Review Then Pass",
			expectedResult: layer4.NeedsReview,
			evals: []*layer4.ControlEvaluation{
				needsReviewEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:       "Needs Review Then Fail",
			expectedResult: layer4.Failed,
			evals: []*layer4.ControlEvaluation{
				needsReviewEvaluation(),
				failingEvaluation(),
			},
		},
		{
			testName:           "Corrupt, Pass, Pass",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
				passingEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:           "Pass, Corrupt, Pass",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				corruptedEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:           "Pass, Pass, Corrupt",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				passingEvaluation(),
				corruptedEvaluation(),
			},
		},
		{
			testName:           "Corrupt, Corrupt, Pass",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
				corruptedEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:           "Corrupt, Corrupt, Corrupt",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
				corruptedEvaluation(),
				corruptedEvaluation(),
			},
		},
		{
			testName:           "Corrupt then Needs Review",
			expectedResult:     layer4.Unknown,
			expectedCorruption: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
				needsReviewEvaluation(),
			},
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(tt *testing.T) {

			v := NewVessel("test", nil, []string{})
			v.config = setSimpleConfig()

			catalogName := strings.Replace(test.testName, " ", "-", -1)
			v.AddEvaluationSuite(catalogName, examplePayload, test.evals)

			// Nothing from our test data should be applicable right now, but they should be possible
			err := v.Mobilize()
			if err != nil {
				tt.Errorf("Expected no error, but got %v", err)
			}
			if v.possibleSuites == nil || len(v.possibleSuites) == 0 {
				tt.Errorf("Expected evaluation suites to be set, but got %v", v.possibleSuites)
				return
			}
			if len(v.Evaluation_Suites) > 0 {
				tt.Errorf("Expected no Evaluation Suites to be set, but got %v", len(v.possibleSuites))
				return
			}

			// Now we set the catalog to be applicable, then run Mobilize again to find results
			v.config.Policy.ControlCatalogs = []string{catalogName}
			v.Mobilize()
			if v.Evaluation_Suites == nil || len(v.Evaluation_Suites) == 0 {
				tt.Errorf("Expected evaluation suites to be set, but got %v", v.Evaluation_Suites)
				return
			}

			for _, suite := range v.Evaluation_Suites {
				tt.Run("suite", func(ttt *testing.T) {
					if len(test.evals) != len(suite.Control_Evaluations) {
						ttt.Errorf("Expected %v control evaluations, but got %v", len(test.evals), len(v.Evaluation_Suites))
					}
					if test.expectedResult != suite.Result {
						ttt.Errorf("Expected result to be %v, but got %v", test.expectedResult, suite.Result)
					}
					if suite.Corrupted_State != test.expectedCorruption {
						ttt.Errorf("Expected corrupted state to be %v, but got %v", test.expectedCorruption, suite.Corrupted_State)
					}
					evaluationSuiteName := fmt.Sprintf("%s_%s", v.Service_Name, catalogName)
					if suite.Name != evaluationSuiteName {
						ttt.Errorf("Expected evaluation suite name to be %s, but got %s", evaluationSuiteName, suite.Name)
					}
					for _, evaluatedSuite := range v.Evaluation_Suites {
						if len(suite.Control_Evaluations) != len(evaluatedSuite.Control_Evaluations) {
							ttt.Errorf("Expected control evaluations to match test data, but got %v", evaluatedSuite.Control_Evaluations)
						}
						testPayloadData := testPayload.(PayloadTypeExample)
						suitePayloadData := evaluatedSuite.payload.(PayloadTypeExample)
						if testPayloadData != suitePayloadData {
							ttt.Errorf("Expected payload to be %v, but got %v", testPayloadData, suitePayloadData)
						}
						if evaluatedSuite.config != v.config {
							ttt.Errorf("Expected config to match simpleConfig but got %v", evaluatedSuite.config)
						}
					}
				})
			}
		})
	}
}
