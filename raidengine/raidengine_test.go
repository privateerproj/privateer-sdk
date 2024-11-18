package raidengine

import (
	"fmt"
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

func (a *testArmory) SetLogger(loggerName string) hclog.Logger {
	a.logger = GetLogger(loggerName, false)
	return a.logger
}

func (a *testArmory) GetTactics() map[string][]Strike {
	a.tactics = map[string][]Strike{
		"passTactic": {
			func() (string, StrikeResult) {
				return "passStrike", StrikeResult{Passed: true}
			},
		},
		"failTactic": {
			func() (string, StrikeResult) {
				return "failStrike", StrikeResult{Passed: false}
			},
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
}{
	{
		testName: "No tacticNames specified",
		armory:   &testArmory{},
		runErr:   "no tactics were specified in the config for the raid 'No_tacticNames_specified'",
	},
	{
		testName:   "Single tacticName specified as 'tactic'",
		tacticName: "testTactic",
		armory:     &testArmory{},
	},
	{
		testName:    "Single tacticName specified in 'tactics' slice",
		tacticNames: []string{"testTactic"},
		armory:      &testArmory{},
	},
	{
		testName:    "Multiple tacticNames specified in 'tactics' slice",
		tacticNames: []string{"passTactic", "nonTactic"},
		armory:      &testArmory{},
	},
	{
		testName:   "A test in the tactic fails",
		tacticName: "failTactic",
		armory:     &testArmory{},
		runErr:     "[A_test_in_the_tactic_fails-failTactic: 0/1 strikes succeeded]",
	},
}

func TestRunTactic(t *testing.T) {
	viper.Set("WriteDirectory", "./tmp")
	for _, tt := range testData {
		raidName := strings.Replace(tt.testName, " ", "_", -1)
		err := runTactic(raidName, tt.tacticName, tt.armory)
		if tt.runErr == "" && err != nil {
			t.Errorf("Expected no error, got %v", err)
		} else if err != nil && err.Error() != tt.runErr {
			t.Errorf("Expected %s, got %v", tt.runErr, err)
		}
	}
}

func TestRun(t *testing.T) {
	viper.Set("WriteDirectory", "./tmp")

	for _, tt := range testData {
		t.Run(tt.testName, func(t *testing.T) {
			raidName := strings.Replace(tt.testName, " ", "_", -1)

			// Haven't managed to viper.Set here without it being overwritten at the beginning of Run(),
			// so we are currently limited on what we can test
			if !strings.Contains(tt.runErr, "no tactics were specified") {
				return
			}

			err := Run(raidName, tt.armory)
			if tt.runErr == "" && err != nil {
				t.Errorf("Expected no error, got %v", err)
			} else if tt.runErr != "" && (err == nil || err.Error() != tt.runErr) {
				t.Errorf("Expected error '%s', got '%v'", tt.runErr, err)
			}
		})
	}
}
