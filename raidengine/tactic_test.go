package raidengine

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

var tacticTestData = []struct {
	testName    string
	raidName    string
	armory      Armory
	runErr      string
	tacticNames []string
	tacticName  string
}{
	{
		testName: "No tacticNames specified",
		raidName: "testRaid",
		armory:   &testArmory{},
	},
	{
		testName:   "Single tacticName specified as 'tactic'",
		raidName:   "testRaid",
		tacticName: "testTactic",
		armory:     &testArmory{},
	},
	{
		testName:    "Single tacticName specified in 'tactics' slice",
		raidName:    "testRaid",
		tacticNames: []string{"testTactic"},
		armory:      &testArmory{},
	},
}

func TestTacticExecute(t *testing.T) {

	// test cases using testArmory and testData from raidengine_test.go
	viper.Set("WriteDirectory", "./tmp")
	for _, tt := range tacticTestData {
		viper.Set(fmt.Sprintf("raids.%s.tactic", tt.raidName), tt.tacticName)
		viper.Set(fmt.Sprintf("raids.%s.tactics", tt.raidName), tt.tacticNames)

		t.Run(tt.testName, func(t *testing.T) {

			viper.Set(fmt.Sprintf("raids.%s.tactics", tt.raidName), tt.tacticNames)

			loggerName = fmt.Sprintf("%s-%s", tt.raidName, tt.tacticName)
			tt.armory.SetLogger(loggerName)
			fmt.Println(loggerName)
			tactic := Tactic{
				TacticName: loggerName,
				strikes:    tt.armory.GetTactics()[tt.tacticName],
			}

			err := tactic.Execute()

			if tt.runErr != "" && (err == nil || err.Error() != tt.runErr) {
				t.Errorf("Expected %v, got %v", tt.runErr, err)
			}

		})
	}
	t.Run("No tacticName specified", func(t *testing.T) {
		tactic := Tactic{}

		err := tactic.Execute()
		if err == nil || err.Error() != "Tactic name was not provided before attempting to execute" {
			t.Errorf("Expected 'Tactic name was not provided before attempting to execute', got nil")
		}
	})
}
