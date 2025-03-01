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
	"strings"
	"syscall"
	"time"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
	"github.com/revanite-io/sci/pkg/layer4"
	"gopkg.in/yaml.v3"
)

type TestSet func() (result layer4.ControlEvaluation)

// TestSuite is a struct that contains the results of all testSets, orgainzed by name
type TestSuite struct {
	TestSuiteName string // TestSuiteName is the name of the TestSuite
	Start_Time    string // Start_Time is the time the plugin started
	End_Time      string // End_Time is the time the plugin ended
	Passed        bool   // Passed is true if all testSets in the testSuite passed
	BadStateAlert bool   // BadState is true if any testSet failed to revert at the end of the testSuite

	Control_Evaluations map[string]layer4.ControlEvaluation // Control_Evaluations is a map of testSet names to their results

	config           *config.Config // config is the global configuration for the plugin
	testSets         []TestSet      // testSets is a list of testSet functions for the current testSuite
	attempts         int            // attempts is the number of testSets attempted
	successes        int            // successes is the number of successful testSets
	failures         int            // failures is the number of failed testSets
	executedTestSets *[]string      // executedTestSets is a list of testSets that have been executed
}

// Execute is used to execute a list of testSets provided by a Plugin and customized by user config
func (t *TestSuite) Execute() error {
	if t.TestSuiteName == "" {
		return errors.New("TestSuite name was not provided before attempting to execute")
	}
	if t.executedTestSets == nil {
		t.executedTestSets = &[]string{}
	}
	t.closeHandler()
	t.Start_Time = time.Now().String()

	for _, testSet := range t.testSets {
		testSetName := getFunctionName(testSet)
		if t.BadStateAlert || utils.StringSliceContains(*t.executedTestSets, testSetName) {
			break
		}
		t.attempts += 1
		testSetResult := testSet()

		testSetResult.Cleanup()

		t.BadStateAlert = testSetResult.Corrupted_State
		logMessage := fmt.Sprintf("%s: %s", testSetResult.Control_Id, testSetResult.Name)
		if testSetResult.Result == layer4.Passed {
			t.successes += 1
			t.config.Logger.Info(logMessage)
		} else {
			t.failures += 1
			t.config.Logger.Error(logMessage)
		}
		t.AddControlEvaluation(testSetName, testSetResult)
	}

	t.cleanup()
	t.End_Time = time.Now().String()

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

// AddControlEvaluation adds a layer4.ControlEvaluation to the TestSuite
func (t *TestSuite) AddControlEvaluation(name string, result layer4.ControlEvaluation) {
	if utils.StringSliceContains(*t.executedTestSets, name) {
		s := append(*t.executedTestSets, name)
		t.executedTestSets = &s
	}

	if t.Control_Evaluations == nil {
		t.Control_Evaluations = make(map[string]layer4.ControlEvaluation)
	}
	t.Control_Evaluations[name] = result
}

func (t *TestSuite) WriteControlEvaluations(serviceName string, output string) error {
	if t.TestSuiteName == "" || serviceName == "" {
		return fmt.Errorf("testSuite name and service name must be provided before attempting to write results: testSuite='%s' service='%s'", t.TestSuiteName, serviceName)
	}

	var err error
	var result []byte
	switch output {
	case "json":
		result, err = json.Marshal(t)
	case "yaml":
		result, err = yaml.Marshal(t)
	default:
		err = fmt.Errorf("output type '%s' is not supported. Supported types are 'json' and 'yaml'", output)
	}
	if err != nil {
		return err
	}

	err = t.writeControlEvaluationsToFile(serviceName, result, output)
	if err != nil {
		return err
	}

	return nil
}

func (t *TestSuite) writeControlEvaluationsToFile(serviceName string, result []byte, extension string) error {
	if !strings.Contains(extension, ".") {
		extension = fmt.Sprintf(".%s", extension)
	}
	dir := path.Join(t.config.WriteDirectory, serviceName)
	filename := fmt.Sprintf("%s%s", t.TestSuiteName, extension)
	filepath := path.Join(dir, filename)

	t.config.Logger.Trace("Writing results", "filepath", filepath)

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			t.config.Logger.Error("Error creating directory", "directory", dir)
			return err
		}
		t.config.Logger.Warn("write directory for this plugin created for results, but should have been created when initializing logs", "directory", dir)
	}

	_, err := os.Create(filepath)
	if err != nil {
		t.config.Logger.Error("Error creating file", "filepath", filepath)
		return err
	}

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		t.config.Logger.Error("Error opening file", "filepath", filepath)
		return err
	}
	defer file.Close()

	_, err = file.Write(result)
	if err != nil {
		return err
	}

	return nil
}

func (t *TestSuite) cleanup() (passed bool) {
	for _, result := range t.Control_Evaluations {
		result.Cleanup()
		t.BadStateAlert = result.Corrupted_State
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
