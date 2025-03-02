package pluginkit

import (
	"testing"

	"github.com/revanite-io/sci/pkg/layer4"
)

func TestCleanup(t *testing.T) {
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			for _, data := range test.data {
				t.Run("subtest_"+data.Name, func(t *testing.T) {
					for _, ce := range data.Control_Evaluations {
						expectedCorrupted := ce.Corrupted_State
						ce.Cleanup()
						if ce.Corrupted_State != expectedCorrupted {
							t.Errorf("Expected control evaluation corruption to be %v, but got %v", expectedCorrupted, ce.Corrupted_State)
						}
						result := data.cleanup()
						if data.Corrupted_State != expectedCorrupted {
							t.Errorf("Expected evaluation suite corruption to be %v, but got %v", expectedCorrupted, data.Corrupted_State)
						}
						if !result != (ce.Corrupted_State || data.Corrupted_State) {
							t.Errorf("Expected cleanup to return %v, but got %v", ce.Corrupted_State || data.Corrupted_State, result)
						}
						if result && !ce.Corrupted_State {
							t.Errorf("Expected control evaluation to be corrupted, but got %v", ce.Corrupted_State)
						}
					}
				})
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			for name, data := range test.data {

				data.config = testingConfig
				err := data.Evaluate("")
				if err.Error() != EVAL_NAME_MISSING().Error() {
					t.Errorf("Expected '%s', but got '%s'", EVAL_NAME_MISSING(), err)
				}

				err = data.Evaluate(test.testName + name)
				if err.Error() != test.expectedEvaluationSuiteError.Error() {
					t.Errorf("Expected %s, but got %s", test.expectedEvaluationSuiteError, err)
				}
				for _, ce := range data.Control_Evaluations {
					if ce.Result == layer4.Passed && !ce.Corrupted_State {
						t.Errorf("Expected control evaluation to be corrupted when result is Passed, but got %v", ce.Corrupted_State)
					}
					// TODO: test more of the evaluation suite behavior
				}
			}
		})
	}
}
