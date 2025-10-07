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

	CorruptedState bool `yaml:"corrupted-state"` // BadState is true if any testSet failed to revert at the end of the evaluation

	EvaluationLog layer4.EvaluationLog `yaml:"control-evaluations"` // EvaluationLog is a slice of evaluations to be executed

	catalog *layer2.Catalog // The Catalog this evaluation suite references

	payload       interface{}    // payload is the data to be evaluated
	loader        DataLoader     // loader is the function to load the payload
	changeManager *ChangeManager // changes is a list of changes made during the evaluation
	config        *config.Config // config is the global configuration for the plugin

	evalSuccesses int // successes is the number of successful evaluations
	evalFailures  int // failures is the number of failed evaluations
	evalWarnings  int // warnings is the number of evaluations that need review
}

// AddChangeManager sets up the change manager for the evaluation suite
func (e *EvaluationSuite) AddChangeManager(cm *ChangeManager) {
	e.changeManager = cm
}

// Execute is used to execute a list of EvaluationLog provided by a Plugin and customized by user config
// Name is an arbitrary string that will be used to identify the EvaluationSuite
func (e *EvaluationSuite) Evaluate(name string) error {
	if name == "" {
		return EVAL_NAME_MISSING()
	}

	if e.config.Invasive && e.changeManager != nil {
		e.changeManager.Allow()
	} else if e.changeManager == nil {
		e.changeManager = &ChangeManager{}
	}

	e.Name = name
	e.StartTime = time.Now().String()
	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.StartTime)

	requirements, err := e.GetAssessmentRequirements()
	if err != nil {
		e.EndTime = time.Now().String()
		return fmt.Errorf("Failed to load assessment requirements from catalog: %w", err)
	}

	for _, evaluation := range e.EvaluationLog.Evaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability)

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = layer4.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.AssessmentLogs {
			message := fmt.Sprintf("%s: %s", assessment.Requirement.EntryId, assessment.Message)
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

			if len(requirements) > 0 && requirements[assessment.Requirement.EntryId] != nil {
				assessment.Recommendation = requirements[assessment.Requirement.EntryId].Recommendation
			}
		}

		if evaluation.Result == layer4.Passed {
			e.evalSuccesses += 1
		} else if evaluation.Result == layer4.Failed {
			e.evalFailures += 1
		} else if evaluation.Result != layer4.NotRun {
			e.evalWarnings += 1
		}
		if e.changeManager.BadState {
			break
		}
	}

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

func (e *EvaluationSuite) GetAssessmentRequirements() (map[string]*layer2.AssessmentRequirement, error) {
	requirements := make(map[string]*layer2.AssessmentRequirement)
	for _, family := range e.catalog.ControlFamilies {
		for _, control := range family.Controls {
			for _, requirement := range control.AssessmentRequirements {
				requirements[requirement.Id] = &requirement
			}
		}
	}

	if len(requirements) == 0 {
		return nil, fmt.Errorf("GetAssessmentRequirements: 0 requirements found")
	}

	return requirements, nil
}
