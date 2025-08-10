package pluginkit

import (
	"fmt"
	"testing"

	"github.com/privateerproj/privateer-sdk/baseline"
	"github.com/revanite-io/sci/layer4"
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
				Name:                test.testName,
				Control_Evaluations: test.evals,
			}
			data.config = setBasicConfig()
			for _, eval := range data.Control_Evaluations {
				expectedCorrupted := eval.Corrupted_State
				eval.Cleanup()
				if eval.Corrupted_State != expectedCorrupted {
					t.Errorf("Expected control evaluation corruption to be %v, but got %v", expectedCorrupted, eval.Corrupted_State)
				}
				result := data.cleanup()
				if result == expectedCorrupted {
					t.Errorf("Expected cleanup to return %v, but got %v", expectedCorrupted, result)
				}
				if data.Corrupted_State != expectedCorrupted {
					t.Errorf("Expected suite result to be %v, but got %v", expectedCorrupted, data.Result)
				}
			}
		})
	}
}


func TestReader (t *testing.T) {

	
	reader := baseline.NewReader()
	if reader == nil {
		t.Fatal("Failed to create Reader instance")
	}

	control, _, err := reader.GetControlByID("OSPS-AC-01")
	if err != nil {
		t.Fatalf("Failed to get control by ID: %v", err)
	}

	fmt.Println(control.AssessmentRequirements)

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
				Name:                test.testName,
				Control_Evaluations: test.evals,
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
			for _, eval := range suite.Control_Evaluations {
				if (eval.Result == layer4.Passed) && eval.Corrupted_State {
					t.Errorf("Control evaluation was marked 'Passed' and Corrupted_State=true")
				}
				// TODO: test more of the evaluation suite behavior
			}
		})
	}
}
