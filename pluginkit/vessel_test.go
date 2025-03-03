package pluginkit

// This file contains table tests for the following functions:
// func (v *Vessel) SetInitilizer(initializer func(*config.Config) error) {
// func (v *Vessel) SetPayload(payload interface{}) {
// func (v *Vessel) Config() *config.Config {
// func (v *Vessel) AddEvaluationSuite(name string, evaluations []layer4.ControlEvaluation) {
// func (v *Vessel) Mobilize(requiredVars []string, suites map[string]EvaluationSuite) error {

import (
	"testing"

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
			name:    "empty payload",
			payload: interface{}(nil),
		},
		{
			name:    "string payload",
			payload: interface{}("test"),
		},
	}

	for _, test := range payloadTestData {
		t.Run(test.name, func(t *testing.T) {
			payload := &test.payload
			v.SetPayload(payload)
			if test.payload != nil && v.Payload.Data != payload {
				t.Errorf("expected payload data to be set to %v, but got %v", payload, v.Payload.Data)
			}
			if v.Payload.Data == nil {
				t.Error("expected v.Payload.Data to never be nil")
			}
		})
	}
}

func TestConfig(t *testing.T) {
	v := NewVessel("test", nil, []string{})
	v.SetupConfig()
	if v.config == nil {
		t.Error("Expected config to be returned")
	}
}

func TestAddEvaluationSuite(t *testing.T) {
	testData := []testingData{
		{
			testName:             "Good Evaluation",
			catalogName:          "catalog1",
			evaluationSuiteName:  "test-service_catalog1",
			applicability:        []string{"valid-applicability-1"},
			expectedSuitesLength: 1,
			expectedResult:       layer4.Passed,
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
			testName:             "Good Evaluation",
			catalogName:          "catalog1",
			evaluationSuiteName:  "test-service_catalog1",
			applicability:        []string{"valid-applicability-1"},
			expectedSuitesLength: 1,
			expectedResult:       layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
		},
		{
			testName:               "Corrupted Evaluation First",
			catalogName:            "catalog1",
			evaluationSuiteName:    "test-service_catalog1",
			applicability:          []string{"valid-applicability-1"},
			expectedSuitesLength:   1,
			expectedResult:         layer4.Unknown,
			expectedCorruptedState: true,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
				passingEvaluation(),
			},
		},
		{
			testName:               "Corrupted Evaluation Second",
			catalogName:            "catalog1",
			evaluationSuiteName:    "test-service_catalog1",
			applicability:          []string{"valid-applicability-1"},
			expectedSuitesLength:   2,
			expectedResult:         layer4.Unknown,
			expectedCorruptedState: true,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				corruptedEvaluation(),
			},
		},
	}

	t.Run("mobilize all testData eval suites", func(t *testing.T) {
		for _, test := range testData {

			v := NewVessel("test", nil, []string{})
			v.config = setSimpleConfig()

			examplePayload := &PayloadTypeExample{CustomPayloadField: true}

			v.AddEvaluationSuite(test.catalogName, examplePayload, test.evals)

			err := v.Mobilize()
			if err != nil {
				t.Errorf("Expected no error, but got %v", err)
			}

			if test.expectedSuitesLength != len(v.Evaluation_Suites) {
				t.Errorf("Expected %v control evaluations, but got %v", len(test.evals), len(v.Evaluation_Suites))
			}

			if v.Evaluation_Suites == nil || len(v.Evaluation_Suites) == 0 {
				continue
			}

			for _, suite := range v.Evaluation_Suites {
				if test.expectedResult != suite.Result {
					t.Errorf("Expected result to be %v, but got %v", test.expectedResult, suite.Result)
				}
				if suite.Corrupted_State != test.expectedCorruptedState {
					t.Errorf("Expected corrupted state to be %v, but got %v", test.expectedCorruptedState, suite.Corrupted_State)
				}
				if suite.Name != test.evaluationSuiteName {
					t.Errorf("Expected evaluation suite name to be %s, but got %s", test.evaluationSuiteName, suite.Name)
				}
				for _, evaluatedSuite := range v.Evaluation_Suites {
					if len(suite.Control_Evaluations) != len(evaluatedSuite.Control_Evaluations) {
						t.Errorf("Expected control evaluations to match test data, but got %v", evaluatedSuite.Control_Evaluations)
					}
					if examplePayload != evaluatedSuite.payload {
						t.Errorf("Expected payload to match test data, but got %v", evaluatedSuite.payload)
					}
					if evaluatedSuite.config != v.config {
						t.Errorf("Expected config to match simpleConfig but got %v", evaluatedSuite.config)
					}
				}
			}
		}
	})
}
