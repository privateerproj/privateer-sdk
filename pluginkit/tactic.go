package pluginkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"reflect"
	"runtime"
	"syscall"
	"time"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
	"gopkg.in/yaml.v3"
)

// Tactic is a struct that contains the results of all strikes, orgainzed by name
type Tactic struct {
	TacticName    string                  // TacticName is the name of the Tactic
	StartTime     string                  // StartTime is the time the plugin started
	EndTime       string                  // EndTime is the time the plugin ended
	StrikeResults map[string]StrikeResult // StrikeResults is a map of strike names to their results
	Passed        bool                    // Passed is true if all strikes in the tactic passed
	BadStateAlert bool                    // BadState is true if any strike failed to revert at the end of the tactic

	config          *config.Config // config is the global configuration for the plugin
	strikes         []Strike       // strikes is a list of strike functions for the current tactic
	attempts        int            // attempts is the number of strikes attempted
	successes       int            // successes is the number of successful strikes
	failures        int            // failures is the number of failed strikes
	executedStrikes *[]string      // executedStrikes is a list of strikes that have been executed
}

// ExecuteTactic is used to execute a list of strikes provided by a Plugin and customized by user config
func (t *Tactic) Execute() error {
	if t.TacticName == "" {
		return errors.New("Tactic name was not provided before attempting to execute")
	}
	if t.executedStrikes == nil {
		t.executedStrikes = &[]string{}
	}
	t.closeHandler()
	t.StartTime = time.Now().String()

	for _, strike := range t.strikes {
		strikeName := getFunctionName(strike)
		if t.BadStateAlert || utils.StringSliceContains(*t.executedStrikes, strikeName) {
			break
		}
		t.attempts += 1
		name, strikeResult := strike()

		strikeResult.followThrough()

		t.BadStateAlert = strikeResult.BadStateAlert
		if strikeResult.Passed {
			t.successes += 1
			t.config.Logger.Info(strikeResult.Message)
		} else {
			t.failures += 1
			t.config.Logger.Error(strikeResult.Message)
		}
		t.AddStrikeResult(name, strikeResult)
	}

	t.cleanup()
	t.EndTime = time.Now().String()

	output := fmt.Sprintf("%s: %v/%v strikes succeeded", t.TacticName, t.successes, t.attempts)
	if t.BadStateAlert {
		return errors.New("!Bad state alert! One or more changes failed to revert. See logs for more information")
	}
	if t.failures == 0 {
		t.Passed = true
		t.config.Logger.Info(output)
		return nil
	}
	return errors.New(output)
}

// AddStrikeResult adds a StrikeResult to the Tactic
func (t *Tactic) AddStrikeResult(name string, result StrikeResult) {
	if utils.StringSliceContains(*t.executedStrikes, name) {
		s := append(*t.executedStrikes, name)
		t.executedStrikes = &s
	}

	if t.StrikeResults == nil {
		t.StrikeResults = make(map[string]StrikeResult)
	}
	t.StrikeResults[name] = result
}

// WriteStrikeResultsJSON unmarhals the Tactic into a JSON file in the user's WriteDirectory
func (t *Tactic) WriteStrikeResultsJSON() error {
	// Log an error if PluginName was not provided
	if t.TacticName == "" {
		return errors.New("Tactic name was not provided before attempting to write results")
	}
	filepath := path.Join(t.config.WriteDirectory, t.TacticName, "results.json")

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		os.MkdirAll(t.config.WriteDirectory, os.ModePerm)
		os.Create(filepath)
	}

	// Write results to file
	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// Marshal results to JSON
	json, err := json.Marshal(t)
	if err != nil {
		return err
	}

	// Write JSON to file
	_, err = file.Write(json)
	if err != nil {
		return err
	}

	return nil
}

// WriteStrikeResultsYAML unmarhals the Tactic into a YAML file in the user's WriteDirectory
func (t *Tactic) WriteStrikeResultsYAML(serviceName string) error {
	// Log an error if PluginName was not provided
	if t.TacticName == "" || serviceName == "" {
		return fmt.Errorf("tactic name and service name must be provided before attempting to write results: tactic='%s' service='%s'", t.TacticName, serviceName)
	}
	dir := path.Join(t.config.WriteDirectory, serviceName)
	filepath := path.Join(dir, t.TacticName+".yml")

	t.config.Logger.Trace(fmt.Sprintf("Writing results to %s", filepath))

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
		t.config.Logger.Error("write directory for this plugin created for results, but should have been created when initializing logs:" + dir)
	}

	os.Create(filepath)

	// Write results to file
	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// Marshal results to YAML
	yaml, err := yaml.Marshal(t)
	if err != nil {
		return err
	}

	// Write YAML to file
	_, err = file.Write(yaml)
	if err != nil {
		return err
	}

	return nil
}

func (t *Tactic) cleanup() (passed bool) {
	for _, result := range t.StrikeResults {
		result.BadStateAlert = revertMovementChanges(&result.Movements)
		t.BadStateAlert = result.BadStateAlert
	}
	return !t.BadStateAlert
}

// closeHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func (t *Tactic) closeHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		t.config.Logger.Error("[ERROR] Execution aborted - %v", "SIGTERM")
		t.config.Logger.Warn("[WARN] Attempting to revert changes made by the terminated Plugin. Do not interrupt this process.")
		if t.cleanup() {
			t.config.Logger.Info("Cleanup did not encounter an error.")
		} else {
			t.config.Logger.Error("[ERROR] Cleanup returned an error, and may not be complete.")
		}
		os.Exit(0)
	}()
}

// getFunctionName returns the name of a function as a string
func getFunctionName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
