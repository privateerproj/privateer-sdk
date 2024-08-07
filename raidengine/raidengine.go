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

// RaidResults is a struct that contains the results of all strikes, orgainzed by name
type RaidResults struct {
	RaidName      string                  // RaidName is the name of the raid
	StartTime     string                  // StartTime is the time the raid started
	EndTime       string                  // EndTime is the time the raid ended
	StrikeResults map[string]StrikeResult // StrikeResults is a map of strike names to their results
}

type Armory interface {
	SetLogger(loggerName string) hclog.Logger
	GetTactics() map[string][]Strike
}

type Strike func() (strikeName string, result StrikeResult)

type cleanupFunc func() error

var logger hclog.Logger
var loggerName string

// cleanup is a function that is called when the program is interrupted
var cleanup = func() error {
	logger.Debug("no custom cleanup specified by this raid, it is likely still under construction")
	return nil
}

func Run(raidName string, strikes Armory) (err error) {
	logger = strikes.SetLogger(raidName)

	tacticsMultiple := fmt.Sprintf("raids.%s.tactics", raidName)
	tacticSingular := fmt.Sprintf("raids.%s.tactic", raidName)

	if viper.IsSet(tacticsMultiple) {
		tactics := viper.GetStringSlice(tacticsMultiple)
		for _, tactic := range tactics {
			strikes.SetLogger(fmt.Sprintf("%s-%s", raidName, tactic))
			viper.Set(tacticSingular, tactic)
			newErr := RunRaid(getStrikes(raidName, strikes.GetTactics()))
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
	strikes.SetLogger(fmt.Sprintf("%s-%s", raidName, viper.GetString(tacticSingular)))
	err = RunRaid(getStrikes(raidName, strikes.GetTactics()))
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

// RunRaid is used to execute a list of strikes provided by a Raid and customize by user config
func RunRaid(strikes []Strike) error {
	closeHandler()

	var attempts int
	var successes int
	var failures int

	raidResults := &RaidResults{
		RaidName:  loggerName,
		StartTime: time.Now().String(),
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
		logger.Info(fmt.Sprintf("%s result:", strikeResult.Message))
		raidResults.AddStrikeResult(name, strikeResult)
	}
	raidResults.EndTime = time.Now().String()
	raidResults.WriteStrikeResultsJSON()
	raidResults.WriteStrikeResultsYAML()
	cleanup()
	output := fmt.Sprintf(
		"%v/%v strikes succeeded. View the output logs for more details.", successes, attempts)
	logger.Info(output)
	if failures > 0 {
		return errors.New(output)
	}
	return nil
}

// GetUniqueStrikes returns a list of unique strikes based on the provided policies
// Strikes listed are unique based on their function address
// Not currently in use. Use this when strike policies are configurable.
func GetUniqueStrikes(strikePacks map[string][]Strike, policies ...string) (strikes []Strike) {
	logger.Debug(fmt.Sprintf(
		"Policies Requested: %s", strings.Join(policies, ",")))

	if len(policies) == 1 {
		// If set via environment variables, this value may come in as a comma delineated string
		policies = strings.Split(policies[0], ",")
	}
	for _, strike := range policies {
		if _, ok := strikePacks[strike]; !ok {
			logger.Error("Strike pack not found for policy: %s (Skipping)", strike)
			continue
		}
		strikes = append(strikes, strikePacks[strike]...)
	}
	return uniqueStrikes(strikes)
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
