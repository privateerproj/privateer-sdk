package pluginkit

import (
	"testing"
)

var testData = []struct {
	testName     string
	expectedPass bool
	value        interface{}
}{
	{
		testName:     "Pass with message and value of type string",
		expectedPass: true,
		value:        "value",
	},
	{
		testName:     "Pass with message and value of type int",
		expectedPass: true,
		value:        1,
	},
	{
		testName:     "Pass with message and value of type string slice",
		expectedPass: true,
		value:        []string{"value"},
	},
	{
		testName:     "Fail with message and value of type string",
		expectedPass: false,
		value:        "value",
	},
	{
		testName:     "Fail with message and value of type int",
		expectedPass: false,
		value:        1,
	},
	{
		testName:     "Fail with message and value of type string slice",
		expectedPass: false,
		value:        []string{"value"},
	},
}

// TestTestResult is a test function for TestResult
func TestTestResult(t *testing.T) {
	t.Run("TestResult", func(t *testing.T) {
		for _, tt := range testData {
			t.Run(tt.testName, func(t *testing.T) {
				testResult := TestResult{}
				testMessage := "this should never change"

				if tt.expectedPass {
					testResult.Pass(testMessage, tt.value)
				} else {
					testResult.Fail(testMessage, tt.value)
				}

				if testResult.Passed != tt.expectedPass {
					t.Errorf("Expected test to pass: %t, got: %t", tt.expectedPass, testResult.Passed)
				}

				if testResult.Message != testMessage {
					t.Errorf("Expected message: %s, got: %s", testMessage, testResult.Message)
				}

				switch testResult.Value.(type) {
				case string:
					if testResult.Value.(string) != tt.value {
						t.Errorf("Expected value: %v, got: %v", tt.value, testResult.Value)
					}
				case int:
					if testResult.Value.(int) != tt.value {
						t.Errorf("Expected value: %v, got: %v", tt.value, testResult.Value)
					}
				case []string:
					if testResult.Value.([]string)[0] != tt.value.([]string)[0] {
						t.Errorf("Expected value: %v, got: %v", tt.value, testResult.Value)
					}

				}
			})
		}
	})
}
