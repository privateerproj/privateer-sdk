package raidengine

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

type testArmory struct {
	logger      hclog.Logger
	serviceName string
	tactics     map[string][]Strike
	raidName    string
	tacticNames []string
	tacticName  string
}

var (
	passStrike = func() (string, StrikeResult) {
		return "passStrike", StrikeResult{Passed: true}
	}

	failStrike = func() (string, StrikeResult) {
		return "failStrike", StrikeResult{Passed: false}
	}

	badStateMovement = func() MovementResult {
		noRevertChange := NewChange(
			"targetName",
			"targetObject",
			func() error { // applyFunc
				return nil
			},
			func() error { // revertFunc
				return errors.New("This pretend change failed to revert")
			},
		)

		noRevertChange.Apply()

		return MovementResult{
			Description: "This movement is still under construction",
			Changes: map[string]*Change{
				"TestChange1": noRevertChange,
			},
		}
	}

	badStateStrike = func() (string, StrikeResult) {
		result := StrikeResult{
			Movements: make(map[string]MovementResult),
		}

		result.ExecuteMovement(badStateMovement)
		return "badStateStrike", result
	}
)

func (a *testArmory) SetLogger(loggerName string) hclog.Logger {
	a.logger = GetLogger(loggerName, false)
	return a.logger
}

func (a *testArmory) GetTactics() map[string][]Strike {
	a.tactics = map[string][]Strike{
		"passTactic": {
			passStrike,
			passStrike,
		},
		"failTactic": {
			passStrike,
			failStrike,
		},
		"badStatePassTactic": {
			passStrike,
			badStateStrike,
		},
		"badStateFailTacticA": {
			passStrike,
			badStateStrike,
		},
		"badStateFailTacticB": {
			badStateStrike,
			passStrike,
		},
	}
	return a.tactics
}

func (a *testArmory) Initialize() error {
	a.TestInit()
	return nil
}

func (a *testArmory) TestInit() {
	viper.Set("WriteDirectory", "./tmp")
}

var testData = []struct {
	testName    string
	armory      Armory
	runErr      string
	tacticNames []string
	tacticName  string
	badState    bool
}{
	{
		testName: "No tacticNames specified",
		armory:   &testArmory{},
		runErr:   "no tactics specified for service No_tacticNames_specified",
	},
	{
		testName:    "Single tacticName specified in tactics slice",
		tacticNames: []string{"testTactic"},
		armory:      &testArmory{},
	},
	{
		testName:    "Multiple tacticNames specified in tactics slice",
		tacticNames: []string{"passTactic", "nonTactic"},
		armory:      &testArmory{},
	},
	{
		testName:    "A test in the tactic fails",
		tacticNames: []string{"failTactic"},
		armory:      &testArmory{},
		runErr:      "A_test_in_the_tactic_fails-failTactic: 1/2 strikes succeeded",
	}, {
		testName:    "A test in the tactic passes and bad state alert is thrown",
		tacticNames: []string{"badStatePassTactic"},
		badState:    true,
		armory:      &testArmory{},
		runErr:      "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},
	{
		testName:    "The last test in the tactic throws a bad state alert",
		tacticNames: []string{"badStateFailTacticA"},
		badState:    true,
		armory:      &testArmory{},
		runErr:      "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},

	{
		// This isn't robustly tested right now, but it is something we want to ensure if we can
		testName:    "A bad state alert prevents the next strike",
		tacticNames: []string{"badStateFailTacticB"},
		badState:    true,
		armory:      &testArmory{},
		runErr:      "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},
}

func TestRunTactic(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.testName, func(t *testing.T) {
			raidName := strings.Replace(tt.testName, " ", "_", -1)

			globalConfig = config.NewConfig(nil) // reset
			globalConfig.WriteDirectory = "./tmp"
			globalConfig.Tactics = tt.tacticNames

			for _, tacticName := range tt.tacticNames {
				err, badStateAlert := runTactic(raidName, tacticName, tt.armory)
				if tt.runErr == "" && err != nil {
					t.Errorf("Expected no error, got %v", err)
				} else if tt.runErr != "" && err == nil {
					t.Errorf("Did not get error, expected '%s' (%s)", tt.runErr, tacticName)
				} else if tt.runErr != "" && err.Error() != tt.runErr {
					t.Errorf("Expected error '%s', got '%v'", tt.runErr, err)
				}
				if tt.badState != badStateAlert {
					t.Errorf("Expected badStateAlert=%v, got badStateAlert=%v (tacticName=%s)", tt.badState, badStateAlert, tacticName)
				}
			}
		})
	}
}

func TestRun(t *testing.T) {

	for _, tt := range testData {
		t.Run(tt.testName, func(t *testing.T) {
			name := strings.Replace(tt.testName, " ", "_", -1)
			viper.Set("service", name)
			viper.Set(fmt.Sprintf("services.%s.tactics", name), tt.tacticNames)

			globalConfig = config.NewConfig(nil) // reset
			globalConfig.WriteDirectory = "./tmp"

			err := Run(name, tt.armory)
			if tt.runErr == "" && err != nil {
				t.Errorf("Expected no error, got %v", err)
			} else if tt.runErr != "" && err == nil {
				t.Errorf("Did not get error, expected '%s'", tt.runErr)
			} else if tt.runErr != "" && err.Error() != tt.runErr {
				t.Errorf("Expected error '%s', got '%v'", tt.runErr, err)
			}
		})
	}
}
