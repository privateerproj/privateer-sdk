package raidengine

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

type Strike func() (strikeName string, result StrikeResult)

// StrikeResult is a struct that contains the results of a check for a single control
type StrikeResult struct {
	Passed      bool                      // Passed is true if the test passed
	Description string                    // Description is a human-readable description of the test
	Message     string                    // Message is a human-readable description of the test result
	DocsURL     string                    // DocsURL is a link to the documentation for the test
	ControlID   string                    // ControlID is the ID of the control that the test is validating
	Movements   map[string]MovementResult // Movements is a list of functions that were executed during the test
}

// ExecuteMovement is a helper function to run a movement function and update the result
func (s *StrikeResult) ExecuteMovement(movementFunc func() MovementResult) {
	// get name of movementFunc as string
	movementFuncName := runtime.FuncForPC(reflect.ValueOf(movementFunc).Pointer()).Name()
	// get the last part of the name, which is the actual function name
	movementName := strings.Split(movementFuncName, ".")[len(strings.Split(movementFuncName, "."))-1]

	movementResult := movementFunc()

	// if this is the first movement or previous movements have passed, accept any results
	if len(s.Movements) == 0 || s.Passed {
		s.Passed = movementResult.Passed
		s.Message = movementResult.Message
	}
	s.Movements[movementName] = movementResult
}

// ExecuteInvasiveMovement is a helper function to run a movement function and update the result
func (s *StrikeResult) ExecuteInvasiveMovement(movementFunc func() MovementResult) {
	if viper.GetBool("invasive") {
		s.ExecuteMovement(movementFunc)
	} else {
		logger.Trace("Invasive movements are disabled, skipping movement")
	}
}

// GetStrikes returns a list of strike objects
func getStrikes(raidName string, tactics map[string][]Strike) []Strike {
	tactic := viper.GetString(fmt.Sprintf("raids.%s.tactic", raidName))
	strikes := tactics[tactic]
	if len(strikes) == 0 {
		message := fmt.Sprintf("No strikes were found for the provided strike set: %s", tactic)
		logger.Error(message)
	}
	return strikes
}

// uniqueStrikes formats the list of unique strikes
// TODO: Decide whether we want to use this for multiple tactic situations
func uniqueStrikes(allStrikes []Strike) (strikes []Strike) {
	used := make(map[string]bool)
	for _, strike := range allStrikes {
		name := getFunctionAddress(strike)
		if _, ok := used[name]; !ok {
			used[name] = true
			strikes = append(strikes, strike)
		}
	}
	return
}

// getFunctionAddress returns the address of a function as a string
func getFunctionAddress(i Strike) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
