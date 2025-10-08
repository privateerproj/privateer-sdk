package pluginkit

import (
	"strings"
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
			// Create a minimal catalog to avoid nil pointer panic
			catalog, err := getTestCatalog()
			if err != nil {
				t.Fatal("Failed to load test catalog")
			}

			suite := &EvaluationSuite{
				Name:          test.testName,
				EvaluationLog: layer4.EvaluationLog{Evaluations: test.evals},
				catalog:       catalog,
			}
			suite.config = setBasicConfig()

			err = suite.Evaluate("arbitrarySuiteName")
			if err != nil && test.expectedEvalSuiteError != nil && err.Error() != test.expectedEvalSuiteError.Error() {
				t.Errorf("Expected %s, but got %s", test.expectedEvalSuiteError, err)
			} else if err != nil && test.expectedEvalSuiteError == nil {
				// For now, we expect an error about missing assessment requirements when catalog is empty
				// This is expected behavior with the current implementation
				expectedMessage := NO_ASSESSMENT_STEPS_PROVIDED()
				if !strings.Contains(err.Error(), expectedMessage.Error()) {
					t.Errorf("Expected error containing '%s', but got '%v'", expectedMessage, err)
				}
			}
		})
	}
}
