package pluginkit

import (
	"fmt"
	"testing"
)

var executeTestTests = []struct {
	testName        string
	expectedPass    bool
	expectedMessage string
	testSetResult   *TestSetResult
	testResult      TestResult
}{
	{
		testName:        "First test passed",
		expectedPass:    true,
		expectedMessage: "Test successful",
		testSetResult: &TestSetResult{
			Message: "No previous tests",
			Tests:   make(map[string]TestResult),
		},
		testResult: TestResult{
			Passed:  true,
			Message: "Test successful",
		},
	},
	{
		testName:        "First test failed",
		expectedPass:    false,
		expectedMessage: "Test failed",
		testSetResult: &TestSetResult{
			Message: "No previous tests",
			Tests:   make(map[string]TestResult),
		},
		testResult: TestResult{
			Passed:  false,
			Message: "Test failed",
		},
	},
	{
		testName:        "Previous test passed-current test passed",
		expectedPass:    true,
		expectedMessage: "Test failed",
		testSetResult: &TestSetResult{
			Passed:  true,
			Message: "Previous test passed",
			Tests: map[string]TestResult{
				"test1": {
					Passed:  true,
					Message: "Test successful",
				},
			},
		},
		testResult: TestResult{
			Passed:  true,
			Message: "Test failed",
		},
	},
	{
		testName:        "Previous test failed-current test passed",
		expectedPass:    false,
		expectedMessage: "Previous test failed",
		testSetResult: &TestSetResult{
			Passed:  false,
			Message: "Previous test failed",
			Tests: map[string]TestResult{
				"test1": {
					Passed:  false,
					Message: "Test failed",
				},
			},
		},
		testResult: TestResult{
			Passed:  true,
			Message: "Test successful",
		},
	},
	{
		testName:        "Previous test passed-current test failed",
		expectedPass:    false,
		expectedMessage: "Test failed",
		testSetResult: &TestSetResult{
			Passed:  true,
			Message: "Previous test passed",
			Tests: map[string]TestResult{
				"test1": {
					Passed:  true,
					Message: "Test successful",
				},
			},
		},
		testResult: TestResult{
			Passed:  false,
			Message: "Test failed",
		},
	},
	{
		testName:        "Previous test failed-current test failed",
		expectedPass:    false,
		expectedMessage: "Previous test failed",
		testSetResult: &TestSetResult{
			Passed:  false,
			Message: "Previous test failed",
			Tests: map[string]TestResult{
				"test1": {
					Passed:  false,
					Message: "Test failed",
				},
			},
		},
		testResult: TestResult{
			Passed:  false,
			Message: "Test failed",
		},
	},
}

func TestExecuteTest(t *testing.T) {
	for _, tt := range executeTestTests {
		t.Run(tt.testName, func(t *testing.T) {
			tt.testSetResult.ExecuteTest(func() TestResult {
				return tt.testResult
			})

			if tt.expectedPass != tt.testSetResult.Passed {
				t.Errorf("testSetResult.Passed = %v, Expected: %v", tt.testSetResult.Passed, tt.expectedPass)
			}
			if tt.expectedMessage != tt.testSetResult.Message {
				t.Errorf("testSetResult.Message = %v, Expected: %v", tt.testSetResult.Message, tt.expectedMessage)
			}
		})
	}
}

func TestExecuteInvasiveTest(t *testing.T) {
	for _, tt := range executeTestTests {
		for _, invasive := range []bool{false, true} {
			// Clone the testSetResult to avoid side effects
			result := &TestSetResult{
				Passed:  tt.testSetResult.Passed,
				Message: tt.testSetResult.Message,
				Tests:   make(map[string]TestResult),
			}
			for k, v := range tt.testSetResult.Tests {
				result.Tests[k] = v
			}

			t.Run(fmt.Sprintf("%s-invasive=%v)", tt.testName, invasive), func(t *testing.T) {
				USER_CONFIG.Invasive = invasive

				// Simulate a test function execution
				result.ExecuteInvasiveTest(func() TestResult {
					return tt.testResult
				})

				if invasive {
					if tt.expectedPass != result.Passed {
						t.Errorf("testSetResult.Passed = %v, Expected: %v", result.Passed, tt.expectedPass)
					}
					if tt.expectedMessage != result.Message {
						t.Errorf("testSetResult.Message = %v, Expected: %v", result.Message, tt.expectedMessage)
					}
				} else {
					if tt.testSetResult.Passed != result.Passed {
						t.Errorf("testSetResult.Passed = %v, Expected: %v", result.Passed, tt.testSetResult.Passed)
					}
					if tt.testSetResult.Message != result.Message {
						t.Errorf("testSetResult.Message = %v, Expected: %v", result.Message, tt.testSetResult.Message)
					}
				}
			})
		}
	}
}
