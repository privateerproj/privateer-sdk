package raidengine

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Tactic is a struct that contains the results of all strikes, orgainzed by name
type Tactic struct {
	TacticName    string                  // TacticName is the name of the Tactic
	StartTime     string                  // StartTime is the time the raid started
	EndTime       string                  // EndTime is the time the raid ended
	StrikeResults map[string]StrikeResult // StrikeResults is a map of strike names to their results

	strikes   []Strike // strikes is a list of strike functions for the current tactic
	attempts  int      // attempts is the number of strikes attempted
	successes int      // successes is the number of successful strikes
	failures  int      // failures is the number of failed strikes
}

// cleanup is a function that is called when the program is interrupted
var cleanup = func() error {
	logger.Debug("no custom cleanup specified by this raid, it is likely still under construction")
	return nil
}

// SetupCloseHandler sets the cleanup function to be called when the program is interrupted
func SetupCloseHandler(customFunction func() error) {
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

// ExecuteTactic is used to execute a list of strikes provided by a Raid and customized by user config
func (t *Tactic) Execute() error {
	if t.TacticName == "" {
		return errors.New("Tactic name was not provided before attempting to execute")
	}
	closeHandler()
	t.StartTime = time.Now().String()

	for _, strike := range t.strikes {
		t.attempts += 1
		name, strikeResult := strike()
		if strikeResult.Message == "" {
			strikeResult.Message = "Strike did not return a result, and may still be under development."
		}
		if strikeResult.Passed {
			t.successes += 1
			logger.Info(strikeResult.Message)
		} else {
			t.failures += 1
			logger.Error(strikeResult.Message)
		}
		t.AddStrikeResult(name, strikeResult)
	}
	t.EndTime = time.Now().String()
	t.WriteStrikeResultsJSON()
	t.WriteStrikeResultsYAML()

	cleanup()

	output := fmt.Sprintf(
		"%s: %v/%v strikes succeeded", t.TacticName, t.successes, t.attempts)
	logger.Info(output)
	if t.failures > 0 {
		return errors.New(output)
	}
	return nil
}
