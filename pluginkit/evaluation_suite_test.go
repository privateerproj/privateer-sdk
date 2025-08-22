package pluginkit

import (
	"testing"

	"github.com/ossf/gemara/layer4"
)

func TestCleanup(t *testing.T) {
	testData := []testingData{
		{
			testName:       "Good Evaluation",
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
		},
		{
			testName:       "Corrupted Evaluation",
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				corruptedEvaluation(),
			},
		},
	}
	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			data := &EvaluationSuite{
				Name:               test.testName,
				ControlEvaluations: test.evals,
			}
			data.config = setBasicConfig()
			for _, eval := range data.ControlEvaluations {
				expectedCorrupted := eval.CorruptedState
				eval.Cleanup()
				if eval.CorruptedState != expectedCorrupted {
					t.Errorf("Expected control evaluation corruption to be %v, but got %v", expectedCorrupted, eval.CorruptedState)
				}
				result := data.cleanup()
				if result == expectedCorrupted {
					t.Errorf("Expected cleanup to return %v, but got %v", expectedCorrupted, result)
				}
				if data.CorruptedState != expectedCorrupted {
					t.Errorf("Expected suite result to be %v, but got %v", expectedCorrupted, data.Result)
				}
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
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
			suite := &EvaluationSuite{
				Name:               test.testName,
				ControlEvaluations: test.evals,
			}
			suite.config = setBasicConfig()
			err := suite.Evaluate("")
			if err.Error() != EVAL_NAME_MISSING().Error() {
				t.Errorf("Expected '%s', but got '%s'", EVAL_NAME_MISSING(), err)
			}

			err = suite.Evaluate("arbitrarySuiteName")
			if err != nil && test.expectedEvalSuiteError != nil && err.Error() != test.expectedEvalSuiteError.Error() {
				t.Errorf("Expected %s, but got %s", test.expectedEvalSuiteError, err)
			}
			for _, eval := range suite.ControlEvaluations {
				if (eval.Result == layer4.Passed) && eval.CorruptedState {
					t.Errorf("Control evaluation was marked 'Passed' and CorruptedState=true")
				}
				// TODO: test more of the evaluation suite behavior
			}
		})
	}
}
