package pluginkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
	"gopkg.in/yaml.v3"
)

type TestSet func() (result layer4.ControlEvaluation)

// TestSuite is a struct that contains the results of all testSets, orgainzed by name
type EvaluationSuite struct {
	Name            string        // Name is the name of the suite
	Start_Time      string        // Start_Time is the time the plugin started
	End_Time        string        // End_Time is the time the plugin ended
	Result          layer4.Result // Result is Passed if all evaluations in the suite passed
	Corrupted_State bool          // BadState is true if any testSet failed to revert at the end of the evaluation

	Control_Evaluations []*layer4.ControlEvaluation // Control_Evaluations is a slice of evaluations to be executed

	payload   *interface{}   // payload is the data to be evaluated
	config    *config.Config // config is the global configuration for the plugin
	successes int            // successes is the number of successful evaluations
	failures  int            // failures is the number of failed evaluations
}

// Execute is used to execute a list of testSets provided by a Plugin and customized by user config
func (e *EvaluationSuite) Evaluate(name string) error {
	if name == "" {
		return EVAL_NAME_MISSING()
	}
	e.Name = name
	e.Start_Time = time.Now().String()
	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.Start_Time)
	for _, evaluation := range e.Control_Evaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability)
		evaluation.Cleanup()

		e.Corrupted_State = evaluation.Corrupted_State
		e.Result = layer4.UpdateAggregateResult(e.Result, evaluation.Result)
		if evaluation.Result == layer4.Passed {
			e.successes += 1
			e.config.Logger.Info(evaluation.Result.String(), "name", e.Name, "message", evaluation.Message)
		} else {
			e.failures += 1
			e.config.Logger.Error(evaluation.Result.String(), "name", e.Name, "message", evaluation.Message)
		}
	}

	e.cleanup()
	e.End_Time = time.Now().String()

	output := fmt.Sprintf("%s: %v/%v control evaluations passed", e.Name, e.successes, len(e.Control_Evaluations))
	if e.Corrupted_State {
		return CORRUPTION_FOUND()
	}
	if e.Result == 0 {
		e.config.Logger.Info(output)
		return nil
	}
	return errors.New(output)
}

func (e *EvaluationSuite) WriteControlEvaluations(serviceName string, output string) error {
	if e.Name == "" || serviceName == "" {
		return fmt.Errorf("testSuite name and service name must be provided before attempting to write results: testSuite='%s' service='%s'", e.Name, serviceName)
	}

	var err error
	var result []byte
	switch output {
	case "json":
		result, err = json.Marshal(e)
	case "yaml":
		result, err = yaml.Marshal(e)
	default:
		err = fmt.Errorf("output type '%s' is not supported. Supported types are 'json' and 'yaml'", output)
	}
	if err != nil {
		return err
	}

	err = e.writeControlEvaluationsToFile(serviceName, result, output)
	if err != nil {
		return err
	}

	return nil
}

func (e *EvaluationSuite) writeControlEvaluationsToFile(serviceName string, result []byte, extension string) error {
	if !strings.Contains(extension, ".") {
		extension = fmt.Sprintf(".%s", extension)
	}
	dir := path.Join(e.config.WriteDirectory, serviceName)
	filename := fmt.Sprintf("%s%s", e.Name, extension)
	filepath := path.Join(dir, filename)

	e.config.Logger.Trace("Writing results", "filepath", filepath)

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			e.config.Logger.Error("Error creating directory", "directory", dir)
			return err
		}
		e.config.Logger.Warn("write directory for this plugin created for results, but should have been created when initializing logs", "directory", dir)
	}

	_, err := os.Create(filepath)
	if err != nil {
		e.config.Logger.Error("Error creating file", "filepath", filepath)
		return err
	}

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		e.config.Logger.Error("Error opening file", "filepath", filepath)
		return err
	}
	defer file.Close()

	_, err = file.Write(result)
	if err != nil {
		return err
	}

	return nil
}

func (e *EvaluationSuite) cleanup() (passed bool) {
	for _, result := range e.Control_Evaluations {
		result.Cleanup()
		e.Corrupted_State = result.Corrupted_State
	}
	return !e.Corrupted_State
}

// closeHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
// Ref: https://golangcode.com/handle-ctrl-c-exit-in-terminal/
func (e *EvaluationSuite) closeHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		e.config.Logger.Error("[ERROR] Execution aborted - %v", "SIGTERM")
		e.config.Logger.Warn("[WARN] Attempting to revert changes made by the terminated Plugin. Do not interrupt this process.")
		if e.cleanup() {
			e.config.Logger.Info("Cleanup did not encounter an error.")
			os.Exit(0)
		} else {
			e.config.Logger.Error("[ERROR] Cleanup returned an error, and may not be complete.")
			os.Exit(2)
		}
	}()
}
