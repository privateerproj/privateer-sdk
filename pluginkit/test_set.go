package pluginkit

import (
	"log"
	"reflect"
	"runtime"
	"strings"

	"github.com/privateerproj/privateer-sdk/utils"
)

type TestSet func() (testSetName string, result TestSetResult)

// TestSetResult is a struct that contains the results of a check for a single control
type TestSetResult struct {
	Passed        bool                  `json:"passed"`        // Passed is true if the test passed
	Description   string                `json:"description"`   // Description is a human-readable description of the test
	Message       string                `json:"message"`       // Message is a human-readable description of the test result
	DocsURL       string                `json:"docsURL"`       // DocsURL is a link to the documentation for the test
	ControlID     string                `json:"controlID"`     // ControlID is the ID of the control that the test is validating
	Tests         map[string]TestResult `json:"tests"`         // Tests is a list of functions that were executed during the test
	BadStateAlert bool                  `json:"badStateAlert"` // BadStateAlert is true if any change failed to revert at the end of the testSet
}

func (s *TestSetResult) followThrough() {
	if s.Message == "" {
		s.Message = "testSet did not return a result, and may still be under development"
	}
	badStateAlert := revertTestChanges(&s.Tests)
	if badStateAlert {
		s.BadStateAlert = true
		s.Message = "One or more changes failed to revert, and the system may be in a bad state. See logs or test details for more information."
		log.Printf("[ERROR] TestSet failed to revert changes, halting execution to prevent further impact")
	}
}

// ExecuteTest is a helper function to run a test function and update the result
func (s *TestSetResult) ExecuteTest(testFunc func() TestResult) {
	// get name of testFunc as string
	testFuncName := runtime.FuncForPC(reflect.ValueOf(testFunc).Pointer()).Name()
	// get the last part of the name, which is the actual function name
	testName := strings.Split(testFuncName, ".")[len(strings.Split(testFuncName, "."))-1]

	testResult := testFunc()

	// if this is the first test or previous tests have passed, accept any results
	if len(s.Tests) == 0 || s.Passed {
		s.Passed = testResult.Passed
		s.Message = testResult.Message
	}
	s.Tests[testName] = testResult
}

// ExecuteInvasiveTest is a helper function to run a test function and update the result
func (s *TestSetResult) ExecuteInvasiveTest(testFunc func() TestResult) {
	if USER_CONFIG.Invasive {
		s.ExecuteTest(testFunc)
	} else {
		log.Printf("[Trace] Invasive tests are disabled, skipping test: %s", utils.CallerName(0))
	}
}
