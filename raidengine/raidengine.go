package raidengine

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

// MovementResult is a struct that contains the results of a single step within a strike
type MovementResult struct {
	Passed      bool        // Passed is true if the test passed
	Description string      // Description is a human-readable description of the test
	Message     string      // Message is a human-readable description of the test result
	Function    string      // Function is the name of the code that was executed
	Value       interface{} // Value is the object that was returned during the movement
}

type Armory interface {
	SetLogger(loggerName string) hclog.Logger
	GetTactics() map[string][]Strike
	Initialize() error // For any custom startup logic, such as bulk config handling
}

var logger hclog.Logger
var loggerName string // This is used for setting up the CLI logger as well as initializing the output logs

// Run executes the raid with the given name using the provided armory.
// It initializes the armory and then executes the tactics specified in the configuration.
// If multiple tactics are specified, it runs each tactic in sequence.
// If an error occurs during initialization or execution of any tactic, it logs the error and returns it.
//
// Parameters:
//   - raidName: The name of the raid to execute.
//   - armory: The armory to use for the raid.
//
// Returns:
//   - err: An error if any occurred during initialization or execution of the raid.
func Run(raidName string, armory Armory) (err error) {
	logger = armory.SetLogger(raidName)
	err = armory.Initialize()
	if err != nil {
		logger.Error("Error initializing the raid armory: %v", err.Error())
		return err
	}
	tacticNames := viper.GetStringSlice(fmt.Sprintf("raids.%s.tactics", raidName))
	tacticName := viper.GetString(fmt.Sprintf("raids.%s.tactics", raidName))

	if len(tacticNames) > 0 {
		// Multiple tactics are specified
		for _, tacticName := range tacticNames {
			runTactic(raidName, tacticName, armory)
		}
		return err
	} else if tacticName != "" {
		// Single tactic is specified, and multiple are NOT specified
		err = runTactic(raidName, tacticName, armory)
	} else {
		err = fmt.Errorf("no tactics were specified in the config for the raid '%s'", raidName)
		logger.Error(err.Error())
	}
	return
}

// runTactic sets the tactic for a given raid, configures the logger, and executes the tactic.
// If an error occurs during execution, it is returned.
//
// Parameters:
//
//	raidName: The name of the raid.
//	tacticName: The name of the tactic to be executed.
//	armory: The Armory instance used to set the logger and retrieve tactics.
//
// Returns:
//
//	err: An error if the tactic execution fails, otherwise nil.
func runTactic(raidName string, tacticName string, armory Armory) (err error) {
	loggerName = fmt.Sprintf("%s-%s", raidName, tacticName)
	armory.SetLogger(loggerName)

	tactic := Tactic{
		TacticName: loggerName,
		strikes:    armory.GetTactics()[tacticName],
	}

	newErr := tactic.Execute()
	if newErr != nil {
		if err != nil {
			err = fmt.Errorf("%s\n%s", err.Error(), newErr.Error())
		} else {
			err = newErr
		}
	}
	return
}
