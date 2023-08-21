package raidengine

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/privateerproj/privateer-sdk/utils"
)

// StrikeResult is a struct that contains the results of a test
type StrikeResult struct {
	Passed      bool   // Passed is true if the test passed
	Description string // Description is a human-readable description of the test
	Message     string // Message is a human-readable description of the test result
	DocsURL     string // DocsURL is a link to the documentation for the test
	ControlID   string // ControlID is the ID of the control that the test is validating
}

// RaidResults is a struct that contains the results of all strikes, orgainzed by name
type RaidResults struct {
	RaidName      string                  // RaidName is the name of the raid
	StartTime     string                  // StartTime is the time the raid started
	EndTime       string                  // EndTime is the time the raid ended
	StrikeResults map[string]StrikeResult // StrikeResults is a map of strike names to their results
}
type Strike func() (strikeName string, result StrikeResult)
type cleanupFunc func() error

var logger hclog.Logger

// cleanup is a function that is called when the program is interrupted
// This default behavior will be overriden by SetupCloseHandler if used by a Raid
var cleanup = func() error {
	logger.Debug("No custom cleanup specified by this raid")
	return nil
}

// Run is used to execute a list of strikes provided by a Raid and customize by user config
func Run(raidName string, availableStrikes map[string][]Strike) error {
	logger = GetLogger(raidName, false)
	closeHandler()

	var attempts int
	var failures int
	strikes := availableStrikes["CIS"] // TODO: Make this configurable
	raidResults := &RaidResults{
		RaidName:  raidName,
		StartTime: time.Now().String(),
	}

	for _, strike := range strikes {
		attempts += 1
		name, strikeResult := strike()
		if strikeResult.Message == "" {
			strikeResult.Message = "Strike did not return a result, and may still be under construction."
		}
		if strikeResult.Passed {
			logger.Info(strikeResult.Message)
		} else {
			failures += 1
			logger.Error(strikeResult.Message)
		}
		logger.Info("%s result:", strikeResult.Message)
		raidResults.AddStrikeResult(name, strikeResult)
	}
	raidResults.EndTime = time.Now().String()
	raidResults.WriteStrikeResultsJSON()
	raidResults.WriteStrikeResultsYAML()
	cleanup()
	output := fmt.Sprintf(
		"%v/%v strikes succeeded. View the output logs for more details.", failures, attempts)
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
			log.Print(utils.ReformatError("Strike pack not found for policy: %s (Skipping)", strike))
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
