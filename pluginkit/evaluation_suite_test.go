package pluginkit

import (
	"testing"

	"github.com/ossf/gemara/layer4"
)

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
				Name:          test.testName,
				EvaluationLog: layer4.EvaluationLog{Evaluations: test.evals},
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
		})
	}
}
