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
			v.SetPayload(&test.payload)
			if test.payload == nil && v.Payload.Data != interface{}(nil) {
				t.Errorf("Did not expect payload data to be set, but got %s", v.Payload.Data)
			} else if test.payload != nil && v.Payload.Data != test.payload {
				t.Errorf("Expected payload data to be set to %s, but got %s", test.payload, v.Payload.Data)
			}

			if v.config == nil {
				t.Error("Expected config to be set")
			}

			if v.Payload.logger == nil {
				t.Error("Expected logger to be set")
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
	if v.Config() == nil {
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
					v.AddEvaluationSuite("test", data.Control_Evaluations)
					if v.CatalogEvaluations["test"].Name != "test" {
						t.Errorf("Expected evaluation suite name to be test, but got %s", v.CatalogEvaluations["test"].Name)
					}
					if len(v.CatalogEvaluations["test"].Control_Evaluations) != len(data.Control_Evaluations) {
						t.Errorf("Expected control evaluations to match test data, but got %v", v.CatalogEvaluations["test"].Control_Evaluations)
					}
					if v.CatalogEvaluations["test"].config != testingConfig {
						t.Errorf("Expected config to match testingConfig but got %v", v.CatalogEvaluations["test"].config)
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
				v.AddEvaluationSuite(name, data.Control_Evaluations)
			}
			err := v.Mobilize()
			if err != nil {
				t.Errorf("Expected no error, but got %v", err)
			}
			if v.CatalogEvaluations == nil {
				t.Error("Expected catalog evaluations to be set")
			}
			if len(v.CatalogEvaluations) != len(test.data) {
				t.Errorf("Expected %v control evaluations, but got %v", len(test.data), len(v.CatalogEvaluations))
			}

			for name, data := range test.data {
				if v.CatalogEvaluations[name].Name != name {
					t.Errorf("Expected evaluation suite name to be %s, but got %s", name, v.CatalogEvaluations[name].Name)
				}
				if len(v.CatalogEvaluations[name].Control_Evaluations) != len(data.Control_Evaluations) {
					t.Errorf("Expected control evaluations to match test data, but got %v", v.CatalogEvaluations[name].Control_Evaluations)
				}
				if v.CatalogEvaluations[name].config != testingConfig {
					t.Errorf("Expected config to match testingConfig but got %v", v.CatalogEvaluations[name].config)
				}
			}
		}
	})
}
