package pluginkit

import (
	"errors"
	"testing"

	"github.com/revanite-io/sci/pkg/layer4"
	"github.com/spf13/viper"
)

var (
	goodApplyFunc = func() (*interface{}, error) {
		return nil, nil
	}
	goodRevertFunc = func() error {
		return nil
	}
	badApplyFunc = func() (*interface{}, error) {
		return nil, errors.New("error")
	}
	badRevertFunc = func() error {
		return errors.New("error")
	}
)

func passingTestSet() layer4.ControlEvaluation {
	assessment := layer4.Assessment{
		Result:      layer4.Passed,
		Description: "passing test",
	}
	change := assessment.NewChange("Change1", "Target1", nil, goodApplyFunc, goodRevertFunc)
	change.Apply()
	return layer4.ControlEvaluation{
		Result:      layer4.Passed,
		Message:     "passing testSet",
		Assessments: []layer4.Assessment{assessment},
	}
}

func failingTestSet() layer4.ControlEvaluation {
	assessment := layer4.Assessment{
		Result:      layer4.Failed,
		Description: "failing test",
	}
	change := assessment.NewChange("Change1", "Target1", nil, goodApplyFunc, goodRevertFunc)
	change.Apply()

	return layer4.ControlEvaluation{
		Result:      layer4.Failed,
		Message:     "passing testSet",
		Assessments: []layer4.Assessment{assessment},
	}
}

func passingBadStateAlertTestSet() layer4.ControlEvaluation {
	assessment := layer4.Assessment{
		Result:      layer4.Passed,
		Description: "passing test",
	}
	change := assessment.NewChange("Change1", "Target1", nil, badApplyFunc, badRevertFunc)
	change.Apply()

	return layer4.ControlEvaluation{
		Result:      layer4.Passed,
		Message:     "passing testSet",
		Assessments: []layer4.Assessment{assessment},
	}
}

func failingBadStateAlertTestSet() layer4.ControlEvaluation {
	assessment := layer4.Assessment{
		Result:      layer4.Failed,
		Description: "passing test",
	}
	change := assessment.NewChange("Change1", "Target1", nil, badApplyFunc, badRevertFunc)
	change.Apply()

	return layer4.ControlEvaluation{
		Result:      layer4.Failed,
		Message:     "passing testSet",
		Assessments: []layer4.Assessment{assessment},
	}
}

var goodArmory = &Armory{
	TestSuites: map[string][]TestSet{
		"PassTestSuite":                {passingTestSet},
		"FailTestSuite":                {failingTestSet},
		"PassedBadStateAlertTestSuite": {passingBadStateAlertTestSet},
		"FailedBadStateAlertTestSuite": {failingBadStateAlertTestSet},
	},
}
var goodVessel = Vessel{
	PluginName: "TestPlugin",
}

var tests = []struct {
	name             string
	serviceName      string
	vessel           Vessel
	armory           *Armory
	testSuiteRequest []string
	requiredVars     []string
	expectedError    error
}{
	{
		name:          "missing service and plugin names",
		serviceName:   "",
		vessel:        Vessel{},
		armory:        goodArmory,
		expectedError: errors.New("expected service and plugin names to be set. ServiceName='' PluginName=''"),
	},
	{
		name:          "missing armory",
		serviceName:   "missingArmory",
		vessel:        goodVessel,
		armory:        nil,
		expectedError: errors.New("vessel's Armory field cannot be nil"),
	},
	{
		name:          "missing test-suites",
		serviceName:   "missingTestSuites",
		vessel:        goodVessel,
		armory:        goodArmory,
		expectedError: errors.New("no test suites requested for service in config: "),
	},
	{
		name:          "missing required vars",
		serviceName:   "missingRequiredVars",
		vessel:        goodVessel,
		armory:        goodArmory,
		requiredVars:  []string{"key", "missing1", "missing2"},
		expectedError: errors.New("missing required variables: [missing1 missing2]"),
	},
	{
		name:             "successful mobilization",
		serviceName:      "successfulMobilization",
		vessel:           goodVessel,
		armory:           goodArmory,
		testSuiteRequest: []string{"PassTestSuite"},
	},
	{
		name:             "successful mobilization, with required vars",
		serviceName:      "successfulMobilization",
		vessel:           goodVessel,
		armory:           goodArmory,
		testSuiteRequest: []string{"PassTestSuite"},
		requiredVars:     []string{"key"},
	},
	{
		name:             "successful mobilization, failed testSuite",
		serviceName:      "failedTestSuite",
		vessel:           goodVessel,
		armory:           goodArmory,
		testSuiteRequest: []string{"FailTestSuite"},
		expectedError:    errors.New("FailTestSuite: 0/1 test sets succeeded"),
	},
	{
		name:             "successful mobilization, passing testSuite, bad state alert",
		serviceName:      "failedTestSuiteBadState",
		vessel:           goodVessel,
		armory:           goodArmory,
		testSuiteRequest: []string{"PassedBadStateAlertTestSuite"},
		expectedError:    errors.New("!Bad state alert! One or more changes failed to revert. See logs for more information"),
	},
	{
		name:             "successful mobilization, failed testSuite, bad state alert",
		serviceName:      "failedTestSuiteBadState",
		vessel:           goodVessel,
		armory:           goodArmory,
		testSuiteRequest: []string{"FailedBadStateAlertTestSuite"},
		expectedError:    errors.New("!Bad state alert! One or more changes failed to revert. See logs for more information"),
	},
}

func TestVessel_Mobilize(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Config reading is tested elsewhere, we care about the ingestion of it
			viper.Set("service", tt.serviceName)
			viper.Set("write-directory", "./tmp")
			viper.Set("services."+tt.serviceName+".test-suites", tt.testSuiteRequest)
			viper.Set("services."+tt.serviceName+".vars", map[string]interface{}{"key": "value"})
			tt.vessel.Armory = tt.armory
			tt.vessel.RequiredVars = tt.requiredVars
			err := tt.vessel.Mobilize()

			if tt.expectedError != nil {
				if err == nil {
					t.Errorf("expected error '%v' but got nil", tt.expectedError)
				} else {
					if err.Error() != tt.expectedError.Error() {
						t.Errorf("expected error '%v' but got '%v'", tt.expectedError, err)
					}
				}
			} else if tt.expectedError == nil && err != nil {
				t.Errorf("expected no error, but got '%v'", err)
			}
		})
	}
}
