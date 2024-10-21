package utils

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/raidengine"
	"github.com/spf13/viper"
)

func TestReformatError(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf) // Intercept expected Stderr output
	defer func() {
		log.SetOutput(os.Stderr) // Return to normal Stderr handling after function
	}()

	longString := "Verify that this somewhat long string remains unchanged in the output after being handled"
	err := ReformatError(longString)
	errContainsString := strings.Contains(err.Error(), longString)
	if !errContainsString {
		t.Logf("Test string was not properly included in retured error")
		t.Fail()
	}
}

func TestFindString(t *testing.T) {

	var tests = []struct {
		slice         []string
		val           string
		expectedIndex int
		expectedFound bool
	}{
		{[]string{"the", "and", "for", "so", "go"}, "and", 1, true},
		{[]string{"the", "and", "for", "so", "go"}, "for", 2, true},
		{[]string{"the", "and", "for", "so", "go"}, "in", -1, false},
	}

	for _, c := range tests {

		testName := fmt.Sprintf("FindString(%q,%q) - Expected:%d,%v", c.slice, c.val, c.expectedIndex, c.expectedFound)

		t.Run(testName, func(t *testing.T) {
			actualPosition, actualFound := FindString(c.slice, c.val)

			if actualPosition != c.expectedIndex || actualFound != c.expectedFound {
				t.Errorf("\nCall: FindString(%q,%q)\nResult: %d,%v\nExpected: %d,%v", c.slice, c.val, actualPosition, actualFound, c.expectedIndex, c.expectedFound)
			}
		})
	}
}

func TestReplaceBytesValue(t *testing.T) {

	var tests = []struct {
		bytes          []byte
		oldValue       string
		newValue       string
		expectedResult []byte
	}{
		{[]byte("oldstringhere"), "old", "new", []byte("newstringhere")},                       //Replace a word with no spaces
		{[]byte("oink oink oink"), "k", "ky", []byte("oinky oinky oinky")},                     //Replace a character
		{[]byte("oink oink oink"), "oink", "moo", []byte("moo moo moo")},                       //Replace a word with spaces
		{[]byte("nothing to replace"), "www", "something", []byte("nothing to replace")},       //Nothing to replace due to no match
		{[]byte(""), "a", "b", []byte("")},                                                     //Empty string
		{[]byte("Unicode character: ㄾ"), "Unicode", "Unknown", []byte("Unknown character: ㄾ")}, //String with unicode character
		{[]byte("Unicode character: ㄾ"), "ㄾ", "none", []byte("Unicode character: none")},       //Replace unicode character
	}

	for _, c := range tests {

		testName := fmt.Sprintf("ReplaceBytesValue(%q,%q,%q) - Expected:%q", string(c.bytes), c.oldValue, c.newValue, string(c.expectedResult))

		t.Run(testName, func(t *testing.T) {
			actualResult := ReplaceBytesValue(c.bytes, c.oldValue, c.newValue)

			if string(actualResult) != string(c.expectedResult) {
				t.Errorf("\nCall: ReplaceBytesValue(%q,%q,%q)\nResult: %q\nExpected: %q", string(c.bytes), c.oldValue, c.newValue, string(actualResult), string(c.expectedResult))
			}
		})
	}
}

func TestCallerPath(t *testing.T) {
	type args struct {
		up int
	}
	tests := []struct {
		testName       string
		testArgs       args
		expectedResult string
	}{
		{"CallerPath(%v) - Expected: %q", args{up: 0}, "github.com/privateerproj/privateer-sdk/utils.TestCallerPath.func1"},
		{"CallerPath(%v) - Expected: %q", args{up: 1}, "testing.tRunner"},
	}

	for _, tt := range tests {
		tt.testName = fmt.Sprintf(tt.testName, tt.testArgs, tt.expectedResult)
		t.Run(tt.testName, func(t *testing.T) {
			if got := CallerPath(tt.testArgs.up); got != tt.expectedResult {
				t.Errorf("CallerPath(%v) = %v, Expected: %v", tt.testArgs.up, got, tt.expectedResult)
			}
		})
	}
}

func TestCallerName(t *testing.T) {
	type args struct {
		up int
	}
	tests := []struct {
		testName       string
		testArgs       args
		expectedResult string
	}{
		{"CallerName(%v) - Expected: %q", args{up: 0}, "func1"},
		{"CallerName(%v) - Expected: %q", args{up: 1}, "tRunner"},
		{"CallerName(%v) - Expected: %q", args{up: 2}, "goexit"},
	}
	for _, tt := range tests {
		tt.testName = fmt.Sprintf(tt.testName, tt.testArgs, tt.expectedResult)
		t.Run(tt.testName, func(t *testing.T) {
			if got := CallerName(tt.testArgs.up); got != tt.expectedResult {
				t.Errorf("CallerName(%v) = %v, Expected: %v", tt.testArgs.up, got, tt.expectedResult)
			}
		})
	}
}

func TestReadStaticFile(t *testing.T) {
	// indev 0.0.1 - removed pkger logic here
}

func TestGetExecutableName(t *testing.T) {

	// Get current executable name for test runner
	execAbsPath, _ := os.Executable()
	testExecName := filepath.Base(execAbsPath)
	if ext := filepath.Ext(testExecName); ext != "" {
		testExecName = strings.TrimSuffix(testExecName, ext)
	}

	tests := []struct {
		testName       string
		expectedResult string
	}{
		// Test cases
		{
			testName:       "GetExecutableName_ReturnsNameWithoutExtension",
			expectedResult: testExecName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if got := GetExecutableName(); got != tt.expectedResult {
				t.Errorf("GetExecutableName() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

// TODO: Unreliable... This test is misbehaving on my device where golang was installed by homebrew
// func TestCallerFileLine(t *testing.T) {
// 	type result struct {
// 		file string
// 		line int
// 	}
// 	tests := []struct {
// 		testName       string
// 		expectedResult result
// 	}{
// 		{"CallerFileLine() - Expected: %q", result{file: "/usr/local/go/src/testing/testing.go", line: 1576}},
// 	}
// 	for _, tt := range tests {
// 		tt.testName = fmt.Sprintf(tt.testName, tt.expectedResult)
// 		t.Run(tt.testName, func(t *testing.T) {
// 			if file, line := CallerFileLine(); file != tt.expectedResult.file || line != tt.expectedResult.line {
// 				t.Errorf("CallerFileLine() = %v, %v, Expected: %v", file, line, tt.expectedResult)
// 			}
// 		})
// 	}
// }

func TestGetRequiredVariable(t *testing.T) {
	tests := []struct {
		testName       string
		variableName   string
		setVariable    interface{} // The value to set in viper for the test
		expectedResult interface{} // Expected value returned by the function
		expectedPassed bool        // Expected state of result.Passed
		expectedMsg    string      // Expected result.Message when the variable is missing
	}{
		{
			testName:       "Missing variable",
			variableName:   "missingVariable",
			setVariable:    nil,
			expectedResult: "",
			expectedPassed: false,
			expectedMsg:    "Required variable does not have a value: missingVariable",
		},
		{
			testName:       "Variable present",
			variableName:   "presentVariable",
			setVariable:    "testValue",
			expectedResult: "testValue",
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Setup: Set the variable in viper for the test
			if tt.setVariable != nil {
				viper.Set(tt.variableName, tt.setVariable)
			} else {
				viper.Set(tt.variableName, "")
			}

			// Create a new result for each test to avoid state carry-over
			result := &raidengine.MovementResult{}

			// Call the function being tested
			got := GetRequiredVariable(tt.variableName, result)

			// Check if the returned value is as expected
			if got != tt.expectedResult {
				t.Errorf("GetRequiredVariable(%q) = %v, Expected: %v", tt.variableName, got, tt.expectedResult)
			}

			// Check if the result.Passed is as expected
			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			// Check if the result.Message is as expected (only if result.Passed is false)
			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			// Clean up: Reset viper after each test
			viper.Set(tt.variableName, nil)
		})
	}
}

// // return a dictionary of variables which will need to be type asserted by the caller
// func GetRequiredVariables(variables []string, result *raidengine.MovementResult) (values map[string]interface{}, err error) {
// 	if result == nil {
// 		result = &raidengine.MovementResult{}
// 	}
// 	values = make(map[string]interface{})
// 	finalMessage := "One or more required variables do not have a value:"
// 	for _, varName := range variables {
// 		values[varName] = GetRequiredVariable(varName, result)
// 		finalMessage = fmt.Sprintf("%s %s", finalMessage, varName)
// 	}
// 	result.Message = finalMessage
// 	if !result.Passed {
// 		// use this err if not using the raidengine.MovementResult object
// 		err = errors.New(finalMessage)
// 	}
// 	return
// }

func TestGetRequiredVariables(t *testing.T) {
	tests := []struct {
		testName       string
		variableNames  []string
		setVariables   map[string]interface{} // The values to set in viper for the test
		expectedResult map[string]interface{} // Expected values returned by the function
		expectedPassed bool                   // Expected state of result.Passed
		expectedMsg    string                 // Expected result.Message when a variable is missing
	}{
		{
			testName:      "Missing variable",
			variableNames: []string{"missingVariable"},
			setVariables:  map[string]interface{}{},
			expectedResult: map[string]interface{}{
				"missingVariable": nil,
			},
			expectedPassed: false,
			expectedMsg:    "One or more required variables do not have a value: missingVariable",
		},
		{
			testName:      "Variable present",
			variableNames: []string{"presentVariable"},
			setVariables: map[string]interface{}{
				"presentVariable": "testValue",
			},
			expectedResult: map[string]interface{}{
				"presentVariable": "testValue",
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
		{
			testName:      "Multiple variables",
			variableNames: []string{"var1", "var2", "var3"},
			setVariables: map[string]interface{}{
				"var1": "value1",
				"var2": 2,
				"var3": true,
			},
			expectedResult: map[string]interface{}{
				"var1": "value1",
				"var2": 2,
				"var3": true,
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Setup: Set the variables in viper for the test
			for name, value := range tt.setVariables {
				viper.Set(name, value)
			}

			// Create a new result for each test to avoid state carry-over
			result := &raidengine.MovementResult{}

			// Call the function being tested
			got, err := GetRequiredVariables(tt.variableNames, result)

			if tt.expectedPassed == false && err.Error() != tt.expectedMsg {
				t.Errorf("GetRequiredVariables(%q) = %v, Expected message: %v", tt.variableNames, err, tt.expectedMsg)
			}
			// Check if the returned value is as expected
			for name, value := range tt.expectedResult {
				if got[name] != value {
					t.Errorf("GetRequiredVariables(%q) = %v, Expected: %v", tt.variableNames, got, tt.expectedResult)
				}
			}

			// Check if the result.Passed is as expected
			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			// Check if the result.Message is as expected (only if result.Passed is false)
			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			// Clean up: Reset viper after each test
			for name := range tt.setVariables {
				viper.Set(name, nil)
			}
		})
	}
}

// func GetRequiredString(variableName string, result *raidengine.MovementResult) string {
// 	found := GetRequiredVariable(variableName, result)
// 	if found == nil {
// 		return ""
// 	}
// 	return found.(string)
// }

func TestGetRequiredString(t *testing.T) {
	tests := []struct {
		testName       string
		variableName   string
		setVariable    interface{} // The value to set in viper for the test
		expectedResult string
		expectedPassed bool
		expectedMsg    string
	}{
		{
			testName:       "Missing variable",
			variableName:   "missingVariable",
			setVariable:    nil,
			expectedResult: "",
			expectedPassed: false,
			expectedMsg:    "Required variable does not have a value: missingVariable",
		},
		{
			testName:       "Variable present",
			variableName:   "presentVariable",
			setVariable:    "testValue",
			expectedResult: "testValue",
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Setup: Set the variable in viper for the test
			if tt.setVariable != nil {
				viper.Set(tt.variableName, tt.setVariable)
			} else {
				viper.Set(tt.variableName, "")
			}

			// Create a new result for each test to avoid state carry-over
			result := &raidengine.MovementResult{}

			// Call the function being tested
			got := GetRequiredString(tt.variableName, result)

			// Check if the returned value is as expected
			if got != tt.expectedResult {
				t.Errorf("GetRequiredString(%q) = %v, Expected: %v", tt.variableName, got, tt.expectedResult)
			}

			// Check if the result.Passed is as expected
			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			// Check if the result.Message is as expected (only if result.Passed is false)
			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			// Clean up: Reset viper after each test
			viper.Set(tt.variableName, nil)
		})
	}
}

// func GetRequiredStrings(variableNames []string, result *raidengine.MovementResult) (value []string, err error) {
// 	found, err := GetRequiredVariables(variableNames, result)
// 	if err != nil {
// 		return
// 	}
// 	for _, v := range found {
// 		value = append(value, v.(string))
// 	}
// 	return
// }

func TestGetRequiredStrings(t *testing.T) {
	tests := []struct {
		testName       string
		variableNames  []string
		setVariables   map[string]interface{} // The values to set in viper for the test
		expectedResult map[string]string
		expectedPassed bool
		expectedMsg    string
	}{
		{
			testName:      "Missing variable",
			variableNames: []string{"missingVariable"},
			setVariables:  map[string]interface{}{},
			expectedResult: map[string]string{
				"missingVariable": "",
			},
			expectedPassed: false,
			expectedMsg:    "One or more required variables do not have a value: missingVariable",
		},
		{
			testName:      "Variable present",
			variableNames: []string{"presentVariable"},
			setVariables: map[string]interface{}{
				"presentVariable": "testValue",
			},
			expectedResult: map[string]string{
				"presentVariable": "testValue",
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
		{
			testName:      "Multiple variables",
			variableNames: []string{"var1", "var2", "var3"},
			setVariables: map[string]interface{}{
				"var1": "value1",
				"var2": "value2",
				"var3": "value3",
			},
			expectedResult: map[string]string{
				"var1": "value1",
				"var2": "value2",
				"var3": "value3",
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Setup: Set the variables in viper for the test
			for name, value := range tt.setVariables {
				viper.Set(name, value)
			}

			// Create a new result for each test to avoid state carry-over
			result := &raidengine.MovementResult{}

			// Call the function being tested
			got, err := GetRequiredStrings(tt.variableNames, result)

			if tt.expectedPassed == false && err.Error() != tt.expectedMsg {
				t.Errorf("GetRequiredStrings(%q) = %v, Expected message: %v", tt.variableNames, err, tt.expectedMsg)
			}
			// Check if the returned value is as expected (equalfold?)
			for key, val := range got {
				if val != tt.expectedResult[key] {
					t.Errorf("GetRequiredStrings(%q) = %v, Expected: %v", tt.variableNames, got, tt.expectedResult)
				}
			}

			// Check if the result.Passed is as expected
			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			// Check if the result.Message is as expected (only if result.Passed is false)
			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			// Clean up: Reset viper after each test
			for name := range tt.setVariables {
				viper.Set(name, nil)
			}
		})
	}
}

// func GetRequiredInt(variableName string, result *raidengine.MovementResult) (value int) {
// 	found := GetRequiredVariable(variableName, result)
// 	if found == nil || found == "" {
// 		result.Passed = false
// 		result.Message = fmt.Sprintf("Required variable does not have a value: %v", variableName)
// 	}
// 	value, ok := found.(int)
// 	if !ok && !result.Passed {
// 		result.Passed = false
// 		result.Message = fmt.Sprintf("Required variable is not an int: %v", variableName)
// 	}
// 	return
// }

func TestGetRequiredInt(t *testing.T) {
	tests := []struct {
		testName       string
		variableName   string
		setVariable    interface{} // The value to set in viper for the test
		expectedResult int
		expectedPassed bool
		expectedMsg    string
	}{
		{
			testName:       "Missing variable",
			variableName:   "missingVariable",
			setVariable:    "StringValue",
			expectedResult: 0,
			expectedPassed: false,
			expectedMsg:    "Required variable is not an int: missingVariable",
		},
		{
			testName:       "Variable present",
			variableName:   "presentVariable",
			setVariable:    42,
			expectedResult: 42,
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Setup: Set the variable in viper for the test
			if tt.setVariable != nil {
				viper.Set(tt.variableName, tt.setVariable)
			} else {
				viper.Set(tt.variableName, "")
			}

			// Create a new result for each test to avoid state carry-over
			result := &raidengine.MovementResult{}

			// Call the function being tested
			got := GetRequiredInt(tt.variableName, result)

			// Check if the returned value is as expected
			if got != tt.expectedResult {
				t.Errorf("GetRequiredInt(%q) = %v, Expected: %v", tt.variableName, got, tt.expectedResult)
			}

			// Check if the result.Passed is as expected
			if result.Passed != tt.expectedPassed {
				t.Errorf("GetRequiredInt(%q) result.Passed = %v, Expected: %v (result.Message=%v)", tt.variableName, result.Passed, tt.expectedPassed, result.Message)
			}

			// Check if the result.Message is as expected (only if result.Passed is false)
			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			// Clean up: Reset viper after each test
			viper.Set(tt.variableName, nil)
		})
	}
}

// func GetRequiredInts(variableNames []string, result *raidengine.MovementResult) (value map[string]int, err error) {
// 	found, err := GetRequiredVariables(variableNames, result)
// 	if err != nil {
// 		return
// 	}
// 	for key, val := range found {
// 		intValue, ok := val.(int)
// 		if !ok {
// 			result.Passed = false
// 			result.Message = fmt.Sprintf("Variable %v is not an int", v)
// 			return
// 		}
// 		value[key] = intValue
// 	}
// 	return
// }

func TestGetRequiredInts(t *testing.T) {
	tests := []struct {
		testName       string
		variableNames  []string
		setVariables   map[string]interface{} // The values to set in viper for the test
		expectedResult map[string]int
		expectedPassed bool
		expectedMsg    string
	}{
		{
			testName:      "Missing variable",
			variableNames: []string{"missingVariable"},
			setVariables:  map[string]interface{}{},
			expectedResult: map[string]int{
				"missingVariable": 0,
			},
			expectedPassed: false,
			expectedMsg:    "One or more required variables do not have a value: missingVariable",
		},
		{
			testName:      "Variable present",
			variableNames: []string{"presentVariable"},
			setVariables: map[string]interface{}{
				"presentVariable": 42,
			},
			expectedResult: map[string]int{
				"presentVariable": 42,
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
		{
			testName:      "Multiple variables",
			variableNames: []string{"var1", "var2", "var3"},
			setVariables: map[string]interface{}{
				"var1": 1,
				"var2": 2,
				"var3": 3,
			},
			expectedResult: map[string]int{
				"var1": 1,
				"var2": 2,
				"var3": 3,
			},
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			for name, value := range tt.setVariables {
				viper.Set(name, value)
			}

			result := &raidengine.MovementResult{}

			got, err := GetRequiredInts(tt.variableNames, result)

			if tt.expectedPassed == false && err.Error() != tt.expectedMsg {
				t.Errorf("GetRequiredInts(%q) = %v, Expected message: %v", tt.variableNames, err, tt.expectedMsg)
			}
			for key, val := range got {
				if val != tt.expectedResult[key] {
					t.Errorf("GetRequiredInts(%q) = %v, Expected: %v", tt.variableNames, got, tt.expectedResult)
				}
			}

			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			for name := range tt.setVariables {
				viper.Set(name, nil)
			}
		})
	}
}

// func GetRequiredBool(variableName string, result *raidengine.MovementResult) (value bool) {
// 	found := GetRequiredVariable(variableName, result)
// 	if found == nil {
// 		return false
// 	}
// 	return found.(bool)
// }

func TestGetRequiredBool(t *testing.T) {
	tests := []struct {
		testName       string
		variableName   string
		setVariable    interface{} // The value to set in viper for the test
		expectedResult bool
		expectedPassed bool
		expectedMsg    string
	}{
		{
			testName:       "Missing variable",
			variableName:   "missingVariable",
			setVariable:    "StringValue",
			expectedResult: false,
			expectedPassed: false,
			expectedMsg:    "Required variable is not a bool: missingVariable",
		},
		{
			testName:       "Variable present",
			variableName:   "presentVariable",
			setVariable:    true,
			expectedResult: true,
			expectedPassed: true,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if tt.setVariable != nil {
				viper.Set(tt.variableName, tt.setVariable)
			} else {
				viper.Set(tt.variableName, "")
			}

			result := &raidengine.MovementResult{}

			got := GetRequiredBool(tt.variableName, result)

			if got != tt.expectedResult {
				t.Errorf("GetRequiredBool(%q) = %v, Expected: %v", tt.variableName, got, tt.expectedResult)
			}

			if result.Passed != tt.expectedPassed {
				t.Errorf("result.Passed = %v, Expected: %v", result.Passed, tt.expectedPassed)
			}

			if result.Passed == false && result.Message != tt.expectedMsg {
				t.Errorf("result.Message = %q, Expected: %q", result.Message, tt.expectedMsg)
			}

			viper.Set(tt.variableName, nil)
		})
	}
}
