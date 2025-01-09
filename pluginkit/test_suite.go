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

// TestSuite is a struct that contains the results of all testSets, orgainzed by name
type TestSuite struct {
	TestSuiteName  string                   // TestSuiteName is the name of the TestSuite
	StartTime      string                   // StartTime is the time the plugin started
	EndTime        string                   // EndTime is the time the plugin ended
	TestSetResults map[string]TestSetResult // TestSetResults is a map of testSet names to their results
	Passed         bool                     // Passed is true if all testSets in the testSuite passed
	BadStateAlert  bool                     // BadState is true if any testSet failed to revert at the end of the testSuite

	config           *config.Config // config is the global configuration for the plugin
	testSets         []TestSet      // testSets is a list of testSet functions for the current testSuite
	attempts         int            // attempts is the number of testSets attempted
	successes        int            // successes is the number of successful testSets
	failures         int            // failures is the number of failed testSets
	executedTestSets *[]string      // executedTestSets is a list of testSets that have been executed
}

// ExecuteTestSuite is used to execute a list of testSets provided by a Plugin and customized by user config
func (t *TestSuite) Execute() error {
	if t.TestSuiteName == "" {
		return errors.New("TestSuite name was not provided before attempting to execute")
	}
	if t.executedTestSets == nil {
		t.executedTestSets = &[]string{}
	}
	t.closeHandler()
	t.StartTime = time.Now().String()

	for _, testSet := range t.testSets {
		testSetName := getFunctionName(testSet)
		if t.BadStateAlert || utils.StringSliceContains(*t.executedTestSets, testSetName) {
			break
		}
		t.attempts += 1
		name, testSetResult := testSet()

		testSetResult.followThrough()

		t.BadStateAlert = testSetResult.BadStateAlert
		logMessage := fmt.Sprintf("%s: %s", testSetResult.ControlID, testSetResult.Message)
		if testSetResult.Passed {
			t.successes += 1
			t.config.Logger.Info(logMessage)
		} else {
			t.failures += 1
			t.config.Logger.Error(logMessage)
		}
		t.AddTestSetResult(name, testSetResult)
	}

	t.cleanup()
	t.EndTime = time.Now().String()

	output := fmt.Sprintf("%s: %v/%v test sets succeeded", t.TestSuiteName, t.successes, t.attempts)
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

// AddTestSetResult adds a TestSetResult to the TestSuite
func (t *TestSuite) AddTestSetResult(name string, result TestSetResult) {
	if utils.StringSliceContains(*t.executedTestSets, name) {
		s := append(*t.executedTestSets, name)
		t.executedTestSets = &s
	}

	if t.TestSetResults == nil {
		t.TestSetResults = make(map[string]TestSetResult)
	}
	t.TestSetResults[name] = result
}

// WriteTestSetResultsJSON unmarhals the TestSuite into a JSON file in the user's WriteDirectory
func (t *TestSuite) WriteTestSetResultsJSON() error {
	// Log an error if PluginName was not provided
	if t.TestSuiteName == "" {
		return errors.New("TestSuite name was not provided before attempting to write results")
	}
	filepath := path.Join(t.config.WriteDirectory, t.TestSuiteName, "results.json")

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

// WriteTestSetResultsYAML unmarhals the TestSuite into a YAML file in the user's WriteDirectory
func (t *TestSuite) WriteTestSetResultsYAML(serviceName string) error {
	// Log an error if PluginName was not provided
	if t.TestSuiteName == "" || serviceName == "" {
		return fmt.Errorf("testSuite name and service name must be provided before attempting to write results: testSuite='%s' service='%s'", t.TestSuiteName, serviceName)
	}
	dir := path.Join(t.config.WriteDirectory, serviceName)
	filepath := path.Join(dir, t.TestSuiteName+".yml")

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

func (t *TestSuite) cleanup() (passed bool) {
	for _, result := range t.TestSetResults {
		result.BadStateAlert = revertTestChanges(&result.Tests)
		t.BadStateAlert = result.BadStateAlert
	}
	return !t.BadStateAlert
}

// closeHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func (t *TestSuite) closeHandler() {
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
