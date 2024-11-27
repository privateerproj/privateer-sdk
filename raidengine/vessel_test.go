package raidengine

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
)

func passStrike() (string, StrikeResult) {
	return "passStrike", StrikeResult{
		Passed:      true,
		Description: "passing strike",
		Movements: map[string]MovementResult{
			"Movement1": {
				Passed:      true,
				Description: "passing movement",
				Changes: map[string]*Change{
					"Change1": {
						Applied:    true,
						applyFunc:  func() error { return nil },
						revertFunc: func() error { return nil },
					},
				},
			},
		},
	}
}

func failStrike() (string, StrikeResult) {
	return "failStrike", StrikeResult{
		Passed:      false,
		Description: "failing strike",
		Movements: map[string]MovementResult{
			"Movement1": {
				Passed:      false,
				Description: "failing movement",
				Changes: map[string]*Change{
					"Change1": {
						Applied:    true,
						applyFunc:  func() error { return nil },
						revertFunc: func() error { return nil },
					},
				},
			},
		},
	}
}

func passBadStateAlertStrike() (string, StrikeResult) {
	return "passBadStateAlertStrike", StrikeResult{
		Passed:      true,
		Description: "passing strike",
		Movements: map[string]MovementResult{
			"Movement1": {
				Passed:      true,
				Description: "passing movement",
				Changes: map[string]*Change{
					"Change1": {
						Applied:    true,
						applyFunc:  func() error { return nil },
						revertFunc: func() error { return errors.New("revert failed") },
					},
				},
			},
		},
	}
}

func failBadStateAlertStrike() (string, StrikeResult) {
	return "failBadStateAlertStrike", StrikeResult{
		Passed:      false,
		Description: "failing strike",
		Movements: map[string]MovementResult{
			"Movement1": {
				Passed:      false,
				Description: "failing movement",
				Changes: map[string]*Change{
					"Change1": {
						Applied:    true,
						applyFunc:  func() error { return nil },
						revertFunc: func() error { return errors.New("revert failed") },
					},
				},
			},
		},
	}
}

var testArmory = Armory{
	RaidName: "TestRaid",
	Tactics: map[string][]Strike{
		"PassTactic":                {passStrike},
		"FailTactic":                {failStrike},
		"PassedBadStateAlertTactic": {passBadStateAlertStrike},
		"FailedBadStateAlertTactic": {failBadStateAlertStrike},
	},
}

var tests = []struct {
	name          string
	serviceName   string
	vessel        Vessel
	armory        *Armory
	tacticRequest []string
	expectedError error
}{
	{
		name:          "missing service and raid names",
		serviceName:   "",
		armory:        &Armory{},
		expectedError: errors.New("expected service and raid names to be set. ServiceName='' RaidName=''"),
	},
	{
		name:          "missing armory",
		serviceName:   "missingArmoryName",
		expectedError: errors.New("armory cannot be nil"),
	},
	{
		name:        "missing tactics",
		serviceName: "missingTactics",
		armory: &Armory{
			RaidName: "TestRaid",
		},
		expectedError: errors.New("no tactics requested for service"),
	},
	{
		name:          "successful mobilization",
		serviceName:   "successfulMobilization",
		armory:        &testArmory,
		tacticRequest: []string{"PassTactic"},
	},
	{
		name:          "successful mobilization, failed tactic",
		serviceName:   "failedTactic",
		armory:        &testArmory,
		tacticRequest: []string{"FailTactic"},
		expectedError: errors.New("failedTactic_FailTactic: 0/1 strikes succeeded"),
	},
	{
		name:          "successful mobilization, passing tactic, bad state alert",
		serviceName:   "failedTacticBadState",
		armory:        &testArmory,
		tacticRequest: []string{"PassedBadStateAlertTactic"},
		expectedError: errors.New("!Bad state alert! One or more changes failed to revert. See logs for more information"),
	},
	{
		name:          "successful mobilization, failed tactic, bad state alert",
		serviceName:   "failedTacticBadState",
		armory:        &testArmory,
		tacticRequest: []string{"FailedBadStateAlertTactic"},
		expectedError: errors.New("!Bad state alert! One or more changes failed to revert. See logs for more information"),
	},
}

func TestVessel_Mobilize(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Config reading is tested elsewhere, we care about the ingestion of it
			viper.Set("service", tt.serviceName)
			viper.Set("write-directory", "./tmp")
			viper.Set("services."+tt.serviceName+".tactics", tt.tacticRequest)

			err := tt.vessel.StockArmory(tt.armory)

			if err == nil {
				err = tt.vessel.Mobilize()
			}

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
