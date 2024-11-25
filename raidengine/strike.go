package raidengine

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
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

	BadStateAlert bool // BadStateAlert is true if any change failed to revert at the end of the strike
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
	if globalConfig.Invasive {
		s.ExecuteMovement(movementFunc)
	} else {
		logger.Trace("Invasive movements are disabled, skipping movement")
	}
}

func (s *StrikeResult) Finalize() {
	if s.Message == "" {
		s.Message = "Strike did not return a result, and may still be under development."
	}
	for movementName, movementResult := range s.Movements {
		for changeName, change := range movementResult.Changes {
			if change.Applied || change.Error != nil {
				if !change.Reverted {
					change.Revert()
				}
				if change.Error != nil || !change.Reverted {
					s.BadStateAlert = true
					logger.Error(fmt.Sprintf("Change in movement '%s' failed to revert. Change name: %s", movementName, changeName))
				}
			}
		}
	}
	if s.BadStateAlert {
		s.Message = "One or more changes failed to revert, and the system may be in a bad state. See logs or movement details for more information."
		logger.Error(fmt.Sprintf("Strike failed to revert changes, halting execution to prevent further impact"))
	}
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
