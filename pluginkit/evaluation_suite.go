package pluginkit

import (
	"fmt"
	"time"

	"github.com/ossf/gemara/layer2"
	"github.com/ossf/gemara/layer4"
	"github.com/privateerproj/privateer-sdk/config"
)

type TestSet func() (result layer4.ControlEvaluation)

// TestSuite is a struct that contains the results of all EvaluationLog, organized by name
type EvaluationSuite struct {
	Name   string        // Name is the name of the suite
	Result layer4.Result // Result is Passed if all evaluations in the suite passed

	CatalogId string `yaml:"catalog-id"` // CatalogId associates this suite with an catalog
	StartTime string `yaml:"start-time"` // StartTime is the time the plugin started
	EndTime   string `yaml:"end-time"`   // EndTime is the time the plugin ended


	EvaluationLog layer4.EvaluationLog `yaml:"control-evaluations"` // EvaluationLog is a slice of evaluations to be executed

	//The plugin will pass us a list of the assessment requirements so that we can build our results, mainly used
	//for populating the recommendation field.
	requirements map[string]*layer2.AssessmentRequirement

	payload interface{}    // payload is the data to be evaluated
	loader  DataLoader     // loader is the function to load the payload
	config  *config.Config // config is the global configuration for the plugin

	evalSuccesses int // successes is the number of successful evaluations
	evalFailures  int // failures is the number of failed evaluations
	evalWarnings  int // warnings is the number of evaluations that need review
}

// Execute is used to execute a list of EvaluationLog provided by a Plugin and customized by user config
// Name is an arbitrary string that will be used to identify the EvaluationSuite
func (e *EvaluationSuite) Evaluate(name string) error {
	if name == "" {
		return EVAL_NAME_MISSING()
	}
	e.Name = name
	e.StartTime = time.Now().String()
	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.StartTime)
	for _, evaluation := range e.EvaluationLog.Evaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability)

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = layer4.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.AssessmentLogs {
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

			//populate the assessment recommendation off of the requirement list passed in (if passed)
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
	}

	e.EndTime = time.Now().String()

	output := fmt.Sprintf("> %s: %v Passed, %v Warnings, %v Failed", e.Name, e.evalSuccesses, e.evalWarnings, e.evalFailures)
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

