package pluginkit

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

var testSuiteTestData = []struct {
	testName    string
	pluginName  string
	armory      *Armory
	runErr      string
	testSuiteNames []string
	testSuiteName  string
}{
	{
		testName:   "No testSuiteNames specified",
		pluginName: "testPlugin",
		armory:     goodArmory,
	},
	{
		testName:   "Single testSuiteName specified as 'test-suite'",
		pluginName: "testPlugin",
		testSuiteName: "testTestSuite",
		armory:     goodArmory,
	},
	{
		testName:    "Single testSuiteName specified in 'test-suites' slice",
		pluginName:  "testPlugin",
		testSuiteNames: []string{"testTestSuite"},
		armory:      goodArmory,
	},
}

func TestTestSuiteExecute(t *testing.T) {

	// test cases using testArmory and testData from pluginkit_test.go
	viper.Set("WriteDirectory", "./tmp")
	for _, tt := range testSuiteTestData {
		viper.Set(fmt.Sprintf("plugins.%s.testSuites", tt.pluginName), tt.testSuiteNames)
		goodVessel.Armory = tt.armory
		goodVessel.StockArmory()

		t.Run(tt.testName, func(t *testing.T) {
			testSuite := TestSuite{
				TestSuiteName: tt.testSuiteName,
				testSets:    tt.armory.TestSuites[tt.testSuiteName],
				config:     tt.armory.Config,
			}
			err := testSuite.Execute()

			if tt.runErr != "" && (err == nil || err.Error() != tt.runErr) {
				t.Errorf("Expected %v, got %v", tt.runErr, err)
			}

		})
	}
	t.Run("No testSuiteName specified", func(t *testing.T) {
		testSuite := TestSuite{}

		err := testSuite.Execute()
		if err == nil || err.Error() != "TestSuite name was not provided before attempting to execute" {
			t.Errorf("Expected 'TestSuite name was not provided before attempting to execute', got nil")
		}
	})
}
