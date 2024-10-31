package raidengine

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

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

// StrikeResult is a struct that contains the results of a check for a single control
type StrikeResult struct {
	Passed      bool                      // Passed is true if the test passed
	Description string                    // Description is a human-readable description of the test
	Message     string                    // Message is a human-readable description of the test result
	DocsURL     string                    // DocsURL is a link to the documentation for the test
	ControlID   string                    // ControlID is the ID of the control that the test is validating
	Movements   map[string]MovementResult // Movements is a list of functions that were executed during the test
}

// Tactic is a struct that contains the results of all strikes, orgainzed by name
type Tactic struct {
	TacticName    string                  // TacticName is the name of the Tactic
	StartTime     string                  // StartTime is the time the raid started
	EndTime       string                  // EndTime is the time the raid ended
	StrikeResults map[string]StrikeResult // StrikeResults is a map of strike names to their results
}

type Armory interface {
	SetLogger(loggerName string) hclog.Logger
	GetTactics() map[string][]Strike
	Initialize() error // For any custom startup logic, such as bulk config handling
}

type Strike func() (strikeName string, result StrikeResult)

type cleanupFunc func() error

var logger hclog.Logger
var loggerName string // This is used for setting up the CLI logger as well as initializing the output logs

// cleanup is a function that is called when the program is interrupted
var cleanup = func() error {
	logger.Debug("no custom cleanup specified by this raid, it is likely still under construction")
	return nil
}

func Run(raidName string, armory Armory) (err error) {
	logger = armory.SetLogger(raidName)
	err = armory.Initialize()
	if err != nil {
		logger.Error("Error initializing the raid armory: %v", err.Error())
		return err
	}

	tacticsMultiple := fmt.Sprintf("raids.%s.tactics", raidName)
	tacticSingular := fmt.Sprintf("raids.%s.tactic", raidName)

	if viper.IsSet(tacticsMultiple) {
		tactics := viper.GetStringSlice(tacticsMultiple)
		for _, tactic := range tactics {
			loggerName = fmt.Sprintf("%s-%s", raidName, tactic)
			armory.SetLogger(loggerName)
			viper.Set(tacticSingular, tactic)
			newErr := ExecuteTactic(getStrikes(raidName, armory.GetTactics()))
			if newErr != nil {
				if err != nil {
					err = fmt.Errorf("%s\n%s", err.Error(), newErr.Error())
				} else {
					err = newErr
				}
			}
		}
		return err
	} else if !viper.IsSet(tacticSingular) {
		err = fmt.Errorf("no tactics were specified in the config for the raid '%s'", raidName)
		logger.Error(err.Error())
		return err
	}

	// In case both 'tactics' and 'tactic' are set in the config for some ungodly reason:
	loggerName := fmt.Sprintf("%s-%s", raidName, viper.GetString(tacticSingular))
	armory.SetLogger(loggerName)
	err = ExecuteTactic(getStrikes(raidName, armory.GetTactics()))
	return err
}

// GetStrikes returns a list of probe objects
func getStrikes(raidName string, tactics map[string][]Strike) []Strike {
	tactic := viper.GetString(fmt.Sprintf("raids.%s.tactic", raidName))
	strikes := tactics[tactic]
	if len(strikes) == 0 {
		message := fmt.Sprintf("No strikes were found for the provided strike set: %s", tactic)
		logger.Error(message)
	}
	return strikes
}

// ExecuteTactic is used to execute a list of strikes provided by a Raid and customized by user config
func ExecuteTactic(strikes []Strike) error {
	closeHandler()

	var attempts int
	var successes int
	var failures int

	tactic := &Tactic{
		TacticName: loggerName,
		StartTime:  time.Now().String(),
	}

	for _, strike := range strikes {
		attempts += 1
		name, strikeResult := strike()
		if strikeResult.Message == "" {
			strikeResult.Message = "Strike did not return a result, and may still be under development."
		}
		if strikeResult.Passed {
			successes += 1
			logger.Info(strikeResult.Message)
		} else {
			failures += 1
			logger.Error(strikeResult.Message)
		}
		tactic.AddStrikeResult(name, strikeResult)
	}
	tactic.EndTime = time.Now().String()
	tactic.WriteStrikeResultsJSON()
	tactic.WriteStrikeResultsYAML()
	cleanup()

	// TODO: This message gets daisy-chained with other raid results... this isn't a good output for chaining like that.
	output := fmt.Sprintf(
		"[%s: %v/%v strikes succeeded]", tactic.TacticName, successes, attempts)
	logger.Info(output)
	if failures > 0 {
		return errors.New(output)
	}
	return nil
}

// uniqueStrikes formats the list of unique strikes
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

// RunMovement is a helper function to run a movement function and update the result
func ExecuteMovement(strikeResult *StrikeResult, movementFunc func() MovementResult) {
	// get name of movementFunc as string
	movementFuncName := runtime.FuncForPC(reflect.ValueOf(movementFunc).Pointer()).Name()
	// get the last part of the name, which is the actual function name
	movementName := strings.Split(movementFuncName, ".")[len(strings.Split(movementFuncName, "."))-1]

	movementResult := movementFunc()

	// if this is the first movement or previous movements have passed, accept any results
	if len(strikeResult.Movements) == 0 || strikeResult.Passed {
		strikeResult.Passed = movementResult.Passed
		strikeResult.Message = movementResult.Message
	}
	strikeResult.Movements[movementName] = movementResult
}

// SetupCloseHandler sets the cleanup function to be called when the program is interrupted
func SetupCloseHandler(customFunction cleanupFunc) {
	cleanup = customFunction
}

// closeHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func closeHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Error("Execution aborted - %v", "SIGTERM")
		if cleanup != nil {
			if err := cleanup(); err != nil {
				logger.Error("Cleanup returned an error, and may not be complete: %v", err.Error())
			}
		} else {
			logger.Trace("No custom cleanup was provided by the terminated Raid.")
		}
		os.Exit(0)
	}()
}
