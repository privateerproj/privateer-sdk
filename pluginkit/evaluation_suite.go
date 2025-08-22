package pluginkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/ossf/gemara/layer2"
	"github.com/ossf/gemara/layer4"
	"github.com/privateerproj/privateer-sdk/config"
)

type TestSet func() (result layer4.ControlEvaluation)

// TestSuite is a struct that contains the results of all ControlEvaluations, organized by name
type EvaluationSuite struct {
	Name   string        // Name is the name of the suite
	Result layer4.Result // Result is Passed if all evaluations in the suite passed

	CatalogId string `yaml:"catalog-id"` // CatalogId associates this suite with an catalog
	StartTime string `yaml:"start-time"` // StartTime is the time the plugin started
	EndTime   string `yaml:"end-time"`   // EndTime is the time the plugin ended

	CorruptedState bool `yaml:"corrupted-state"` // BadState is true if any testSet failed to revert at the end of the evaluation

	ControlEvaluations []*layer4.ControlEvaluation `yaml:"control-evaluations"` // ControlEvaluations is a slice of evaluations to be executed

	//The plugin will pass us a list of the assessment requirements so that we can build our results, mainly used
	//for populating the reccomendation field.
	requirements map[string]*layer2.AssessmentRequirement

	payload interface{}    // payload is the data to be evaluated
	loader  DataLoader     // loader is the function to load the payload
	config  *config.Config // config is the global configuration for the plugin

	evalSuccesses int // successes is the number of successful evaluations
	evalFailures  int // failures is the number of failed evaluations
	evalWarnings  int // warnings is the number of evaluations that need review
}

// Execute is used to execute a list of ControlEvaluations provided by a Plugin and customized by user config
// Name is an arbitrary string that will be used to identify the EvaluationSuite
func (e *EvaluationSuite) Evaluate(name string) error {
	if name == "" {
		return EVAL_NAME_MISSING()
	}
	e.Name = name
	e.StartTime = time.Now().String()
	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.StartTime)
	for _, evaluation := range e.ControlEvaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability, e.config.Invasive)
		evaluation.Cleanup()
		if !e.CorruptedState {
			e.CorruptedState = evaluation.CorruptedState
		}

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = layer4.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.Assessments {
			message := fmt.Sprintf("%s: %s", assessment.RequirementId, assessment.Message)
			// switch case the code below
			switch assessment.Result {
			case layer4.Passed:
				e.config.Logger.Info(message)
			case layer4.NeedsReview:
				e.config.Logger.Warn(message)
			case layer4.Failed:
				e.config.Logger.Error(message)
			case layer4.Unknown:
				e.config.Logger.Error(message)
			}

			//populate the assessment reccomendation off of the requirement list passed in (if passed)
			if len(e.requirements) > 0 && e.requirements[assessment.RequirementId] != nil {
				assessment.Recommendation = e.requirements[assessment.RequirementId].Recommendation
			}
		}

		if evaluation.Result == layer4.Passed {
			e.evalSuccesses += 1
		} else if evaluation.Result == layer4.Failed {
			e.evalFailures += 1
		} else if evaluation.Result != layer4.NotRun {
			e.evalWarnings += 1
		}
		if e.CorruptedState {
			break
		}
	}

	e.cleanup()
	e.EndTime = time.Now().String()

	output := fmt.Sprintf("> %s: %v Passed, %v Warnings, %v Failed", e.Name, e.evalSuccesses, e.evalWarnings, e.evalFailures)
	if e.CorruptedState {
		return CORRUPTION_FOUND()
	}
	switch e.Result {
	case layer4.Passed:
		e.config.Logger.Info(output)
	case layer4.NotRun:
		e.config.Logger.Trace(output)
	default:
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
	defer func() {
		_ = file.Close()
	}()

	_, err = file.Write(result)
	if err != nil {
		return err
	}

	return nil
}

func (e *EvaluationSuite) cleanup() (passed bool) {
	for _, result := range e.ControlEvaluations {
		result.Cleanup()
		if result.CorruptedState {
			e.CorruptedState = result.CorruptedState
		}
	}
	return !e.CorruptedState
}
