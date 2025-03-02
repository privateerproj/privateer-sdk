package pluginkit

// This file contains table tests for the following functions:
// func (v *Vessel) SetInitilizer(initializer func(*config.Config) error) {
// func (v *Vessel) SetPayload(payload *interface{}) {
// func (v *Vessel) Config() *config.Config {
// func (v *Vessel) AddEvaluationSuite(name string, evaluations []layer4.ControlEvaluation) {
// func (v *Vessel) Mobilize(requiredVars []string, suites map[string]EvaluationSuite) error {

import (
	"testing"
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
				t.Errorf("Expected payload data to be set to %v, but got %v", payload, v.Payload.Data)
			}

			if v.config == nil {
				t.Error("Expected config to be set")
			}

			if v.Payload.config == nil {
				t.Error("Expected payload config to be set")
			}

			if v.Payload.config != v.config {
				t.Error("Expected payload config to be the same as vessel config")
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
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			for _, data := range test.data {
				t.Run("subtest_"+data.Name, func(t *testing.T) {
					v := NewVessel("test", nil, []string{})
					v.config = testingConfig
					v.AddEvaluationSuite("test", nil, data.Control_Evaluations)
					if v.possibleSuites == nil || len(v.possibleSuites) == 0 {
						t.Error("Expected evaluation suites to be set")
						return
					}
					if v.possibleSuites[0].Name != "test" {
						t.Errorf("Expected evaluation suite name to be test, but got %s", v.possibleSuites[0].Name)
					}
					if len(v.possibleSuites[0].Control_Evaluations) != len(data.Control_Evaluations) {
						t.Errorf("Expected control evaluations to match test data, but got %v", v.possibleSuites[0].Control_Evaluations)
					}
					if v.possibleSuites[0].config != testingConfig {
						t.Errorf("Expected config to match testingConfig but got %v", v.possibleSuites[0].config)
					}
				})
			}
		})
	}
}

func TestMobilize(t *testing.T) {
	t.Run("mobilize all testData eval suites", func(t *testing.T) {
		for _, test := range testData {

			v := NewVessel("test", nil, []string{})
			v.config = testingConfig

			for name, data := range test.data {
				v.AddEvaluationSuite(name, nil, data.Control_Evaluations)
			}
			err := v.Mobilize()
			if err != nil {
				t.Errorf("Expected no error, but got %v", err)
			}
			if v.Evaluation_Suites == nil {
				t.Error("Expected catalog evaluations to be set")
			}
			if len(v.Evaluation_Suites) != len(test.data) {
				t.Errorf("Expected %v control evaluations, but got %v", len(test.data), len(v.Evaluation_Suites))
			}

			if len(v.Evaluation_Suites) == 0 {
				continue
			}

			for name, data := range test.data {
				if v.Evaluation_Suites[0].Name != name {
					t.Errorf("Expected evaluation suite name to be %s, but got %s", name, v.Evaluation_Suites[0].Name)
				}
				if len(v.Evaluation_Suites[0].Control_Evaluations) != len(data.Control_Evaluations) {
					t.Errorf("Expected control evaluations to match test data, but got %v", v.Evaluation_Suites[0].Control_Evaluations)
				}
				if v.Evaluation_Suites[0].config != testingConfig {
					t.Errorf("Expected config to match testingConfig but got %v", v.Evaluation_Suites[0].config)
				}
			}
		}
	})
}
