package pluginkit

import (
	"encoding/json"
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

// TestSuite is a struct that contains the results of all ControlEvaluations, orgainzed by name
type EvaluationSuite struct {
	Name            string        // Name is the name of the suite
	Catalog_Id      string        // Catalog_Id associates this suite with an catalog
	Start_Time      string        // Start_Time is the time the plugin started
	End_Time        string        // End_Time is the time the plugin ended
	Result          layer4.Result // Result is Passed if all evaluations in the suite passed
	Corrupted_State bool          // BadState is true if any testSet failed to revert at the end of the evaluation

	Control_Evaluations []*layer4.ControlEvaluation // Control_Evaluations is a slice of evaluations to be executed

	payload interface{}    // payload is the data to be evaluated
	loader  DataLoader     // loader is the function to load the payload
	config  *config.Config // config is the global configuration for the plugin

	assessmentSuccesses int // successes is the number of successful evaluations
	assessmentWarnings  int // attempts is the number of evaluations attempted
	assessmentFailures  int // failures is the number of failed evaluations
	evalSuccesses       int // successes is the number of successful evaluations
	evalFailures        int // failures is the number of failed evaluations
	evalWarnings        int // warnings is the number of evaluations that need review
}

// Execute is used to execute a list of ControlEvaluations provided by a Plugin and customized by user config
// Name is an arbitrary string that will be used to identify the EvaluationSuite
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
		if !e.Corrupted_State {
			e.Corrupted_State = evaluation.Corrupted_State
		}

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = layer4.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.Assessments {
			message := fmt.Sprintf("%s: %s", assessment.Requirement_Id, assessment.Message)
			if assessment.Result == layer4.Passed {
				e.config.Logger.Info(message)
			} else if assessment.Result == layer4.NeedsReview {
				e.config.Logger.Warn(message)
			} else if assessment.Result == layer4.Failed || assessment.Result == layer4.Unknown {
				e.config.Logger.Error(message)
			}
		}

		if evaluation.Result == layer4.Passed {
			e.evalSuccesses += 1
		} else if evaluation.Result == layer4.Failed {
			e.evalFailures += 1
		} else if evaluation.Result != layer4.NotRun {
			e.evalWarnings += 1
		}
		if e.Corrupted_State {
			break
		}
	}

	e.cleanup()
	e.End_Time = time.Now().String()

	output := fmt.Sprintf("> %s: %v Passed, %v Warnings, %v Failed", e.Name, e.evalSuccesses, e.evalWarnings, e.evalFailures)
	if e.Corrupted_State {
		return CORRUPTION_FOUND()
	}
	if e.Result == layer4.Passed {
		e.config.Logger.Info(output)
	} else if e.Result == layer4.NotRun {
		e.config.Logger.Trace(output)
	} else {
		e.config.Logger.Error(output)
	}
	return nil
}

func (e *EvaluationSuite) WriteControlEvaluations(serviceName string, output string) error {
	if e.Name == "" || serviceName == "" {
		return fmt.Errorf("EvaluationSuite name and service name must be provided before attempting to write results: EvaluationSuite='%s' service='%s'", e.Name, serviceName)
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
		if result.Corrupted_State {
			e.Corrupted_State = result.Corrupted_State
		}
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
