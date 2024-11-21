package raidengine

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

type testArmory struct {
	logger      hclog.Logger
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
		log.Printf("Running badStateMovement\n")
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
	viper.Set(fmt.Sprintf("raids.%s.tactics", a.raidName), a.tacticNames)
	viper.Set(fmt.Sprintf("raids.%s.tactic", a.raidName), a.tacticName)
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
		runErr:   "no tactic was specified for the raid 'No_tacticNames_specified'",
	},
	{
		testName:   "Single tacticName specified as tactic",
		tacticName: "testTactic",
		armory:     &testArmory{},
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
		testName:   "A test in the tactic fails",
		tacticName: "failTactic",
		armory:     &testArmory{},
		runErr:     "A_test_in_the_tactic_fails-failTactic: 1/2 strikes succeeded",
	}, {
		testName:   "A test in the tactic passes and bad state alert is thrown",
		tacticName: "badStatePassTactic",
		badState:   true,
		armory:     &testArmory{},
		runErr:     "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},
	{
		testName:   "The last test in the tactic throws a bad state alert",
		tacticName: "badStateFailTacticA",
		badState:   true,
		armory:     &testArmory{},
		runErr:     "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},

	{
		testName:   "A bad state alert prevents the next tactic",
		tacticName: "badStateFailTacticB",
		badState:   true,
		armory:     &testArmory{},
		runErr:     "!Bad state alert! One or more changes failed to revert. See logs for more information",
	},
}

func TestRunTactic(t *testing.T) {
	viper.Set("WriteDirectory", t.TempDir())
	for _, tt := range testData {
		t.Run(tt.testName, func(t *testing.T) {
			raidName := strings.Replace(tt.testName, " ", "_", -1)
			if tt.tacticName != "" {
				err, badStateAlert := runTactic(raidName, tt.tacticName, tt.armory)
				if tt.runErr == "" && err != nil {
					t.Errorf("Expected no error, got %v", err)
				} else if tt.runErr != "" && err == nil {
					t.Errorf("Did not get error, expected '%s' (%s)", tt.runErr, tt.tacticName)
				} else if tt.runErr != "" && err.Error() != tt.runErr {
					t.Errorf("Expected error '%s', got '%v'", tt.runErr, err)
				}
				if tt.badState != badStateAlert {
					t.Errorf("Expected badStateAlert=%v, got badStateAlert=%v (tacticName=%s)", tt.badState, badStateAlert, tt.tacticName)
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	viper.Set("WriteDirectory", t.TempDir())

	for _, tt := range testData {
		t.Run(tt.testName, func(t *testing.T) {
			raidName := strings.Replace(tt.testName, " ", "_", -1)

			// Haven't managed to viper.Set here without it being overwritten at the beginning of Run(),
			// so we are currently limited on what we can test
			if !strings.Contains(tt.runErr, "no tactic was specified") {
				return
			}

			err := Run(raidName, tt.armory)
			if tt.runErr == "" && err != nil {
				t.Errorf("Expected no error, got %v", err)
			} else if tt.runErr != "" && err == nil {
				t.Errorf("Did not get error, expected '%s' (%s)", tt.runErr, tt.tacticName)
			} else if tt.runErr != "" && err.Error() != tt.runErr {
				t.Errorf("Expected error '%s', got '%v'", tt.runErr, err)
			}
			if tt.badState && !strings.Contains(err.Error(), "!Bad state alert!") {
				t.Errorf("Expected bad state alert, got %v", err)
			}
		})
	}
}
