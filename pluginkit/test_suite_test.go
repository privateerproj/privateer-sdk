package pluginkit

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

var testArmory = &Armory{
	TestSuites: map[string][]TestSet{
		"PassTestSuite":                {passingTestSet},
		"FailTestSuite":                {failingTestSet},
		"PassedBadStateAlertTestSuite": {passingBadStateAlertTestSet},
		"FailedBadStateAlertTestSuite": {failingBadStateAlertTestSet},
	},
}

var testVessel = Vessel{
	PluginName: "TestPlugin",
}

var testSuiteTestData = []struct {
	testName       string
	pluginName     string
	armory         *Armory
	runErr         string
	testSuiteNames []string
	testSuiteName  string
}{
	{
		testName:   "No testSuiteNames specified",
		pluginName: "testPlugin",
		armory:     goodArmory,
	},
	{
		testName:      "Single testSuiteName specified as 'test-suite'",
		pluginName:    "testPlugin",
		testSuiteName: "testTestSuite",
		armory:        goodArmory,
	},
	{
		testName:       "Single testSuiteName specified in 'test-suites' slice",
		pluginName:     "testPlugin",
		testSuiteNames: []string{"testTestSuite"},
		armory:         goodArmory,
	},
}

func TestTestSuiteExecute(t *testing.T) {

	// test cases using testArmory and testData from pluginkit_test.go
	viper.Set("WriteDirectory", "./tmp")
	for _, tt := range testSuiteTestData {
		viper.Set(fmt.Sprintf("plugins.%s.testSuites", tt.pluginName), tt.testSuiteNames)
		testVessel.Armory = tt.armory
		testVessel.StockArmory()

		t.Run(tt.testName, func(t *testing.T) {
			err := testVessel.Evaluate(tt.testSuiteName, nil)
			if err != nil && err.Error() != tt.runErr {
				t.Errorf("Expected '%s', got '%s'", tt.runErr, err.Error())
			}
		})
	}
	t.Run("No testSuiteName specified", func(t *testing.T) {
		e := EvaluationSuite{}

		err := e.Evaluate(nil)
		if err == nil || err.Error() != "EvaluationSuite name was not provided before attempting to execute" {
			t.Errorf("Expected 'EvaluationSuite name was not provided before attempting to execute', got nil")
		}
	})
}
