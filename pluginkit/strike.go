package pluginkit

import (
	"log"
	"reflect"
	"runtime"
	"strings"

	"github.com/privateerproj/privateer-sdk/utils"
)

type Strike func() (strikeName string, result StrikeResult)

// StrikeResult is a struct that contains the results of a check for a single control
type StrikeResult struct {
	Passed        bool                      // Passed is true if the test passed
	Description   string                    // Description is a human-readable description of the test
	Message       string                    // Message is a human-readable description of the test result
	DocsURL       string                    // DocsURL is a link to the documentation for the test
	ControlID     string                    // ControlID is the ID of the control that the test is validating
	Movements     map[string]MovementResult // Movements is a list of functions that were executed during the test
	BadStateAlert bool                      // BadStateAlert is true if any change failed to revert at the end of the strike

	invasivePlugin bool // invasivePlugin is true if the tactic is allowed to make changes to the target service
}

func (s *StrikeResult) followThrough() {
	if s.Message == "" {
		s.Message = "strike did not return a result, and may still be under development"
	}
	badStateAlert := revertMovementChanges(&s.Movements)
	if badStateAlert {
		s.BadStateAlert = true
		s.Message = "One or more changes failed to revert, and the system may be in a bad state. See logs or movement details for more information."
		log.Printf("[ERROR] Strike failed to revert changes, halting execution to prevent further impact")
	}
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
	if s.invasivePlugin {
		s.ExecuteMovement(movementFunc)
	} else {
		log.Printf("[Trace] Invasive movements are disabled, skipping movement: %s", utils.CallerName(0))
	}
}
