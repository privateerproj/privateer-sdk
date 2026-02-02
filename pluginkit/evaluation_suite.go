package pluginkit

import (
	"fmt"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/config"
)

// TestSet is a function type that returns a control evaluation result.
type TestSet func() (result gemara.ControlEvaluation)

// EvaluationSuite contains the results of all EvaluationLog executions.
// Exported fields will be used in the final YAML or JSON output documents.
type EvaluationSuite struct {
	Name   string        // Name is the name of the suite
	Result gemara.Result // Result is Passed if all evaluations in the suite passed

	CatalogId string `yaml:"catalog-id"` // CatalogId associates this suite with a catalog
	StartTime string `yaml:"start-time"` // StartTime is the time the plugin started
	EndTime   string `yaml:"end-time"`   // EndTime is the time the plugin ended

	CorruptedState bool `yaml:"corrupted-state"` // CorruptedState is true if any testSet failed to revert at the end of the evaluation

	EvaluationLog gemara.EvaluationLog `yaml:"control-evaluations"` // EvaluationLog is a slice of evaluations to be executed

	config *config.Config // config is the global configuration

	payload       interface{}                        // payload is the data to be evaluated
	loader        DataLoader                         // loader is the function to load the payload
	changeManager *ChangeManager                     // changes is a list of changes made during the evaluation
	catalog       *gemara.ControlCatalog                    // The Catalog this evaluation suite references
	steps         map[string][]gemara.AssessmentStep // steps is a map of control IDs to their assessment steps

	evalSuccesses int // successes is the number of successful evaluations
	evalFailures  int // failures is the number of failed evaluations
	evalWarnings  int // warnings is the number of evaluations that need review
}

// AddChangeManager sets up the change manager for the evaluation suite.
func (e *EvaluationSuite) AddChangeManager(cm *ChangeManager) {
	if e.config.Invasive && cm != nil {
		e.changeManager = cm
		e.changeManager.Allow()
	}
}

// Evaluate executes a list of EvaluationLog provided by a Plugin and customized by user config.
// Name is an arbitrary string that will be used to identify the EvaluationSuite.
func (e *EvaluationSuite) Evaluate(serviceName string) error {
	if e.config == nil {
		return CONFIG_NOT_INITIALIZED("ev10")
	}

	requirements, err := e.GetAssessmentRequirements()
	if err != nil {
		return BAD_ASSESSMENT_REQS(err, "ev20")
	}

	evalLog, err := e.setupEvalLog(e.steps)
	if err != nil {
		return BAD_EVAL_LOG(err, "ev30")
	}

	if len(evalLog.Evaluations) == 0 {
		return EVAL_SUITE_CRASHED("ev40")
	}

	e.Name = fmt.Sprintf("%s_%s", serviceName, e.CatalogId)
	e.EvaluationLog = evalLog
	e.StartTime = time.Now().String()

	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.StartTime)

	for _, evaluation := range e.EvaluationLog.Evaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability)

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = gemara.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.AssessmentLogs {
			message := fmt.Sprintf("%s: %s", assessment.Requirement.EntryId, assessment.Message)
			// switch case the code below
			switch assessment.Result {
			case gemara.Passed:
				e.config.Logger.Info(message)
			case gemara.NeedsReview:
				e.config.Logger.Warn(message)
			case gemara.Failed:
				e.config.Logger.Error(message)
			case gemara.Unknown:
				e.config.Logger.Error(message)
			}

			if len(requirements) > 0 && requirements[assessment.Requirement.EntryId] != nil {
				assessment.Recommendation = requirements[assessment.Requirement.EntryId].Recommendation
			}
		}

		if evaluation.Result == gemara.Passed {
			e.evalSuccesses += 1
		} else if evaluation.Result == gemara.Failed {
			e.evalFailures += 1
		} else if evaluation.Result != gemara.NotRun {
			e.evalWarnings += 1
		}
		if e.changeManager != nil && e.changeManager.CorruptedState {
			break
		}
	}

	output := fmt.Sprintf("> %s: %v Passed, %v Warnings, %v Failed, %v Possible", e.Name, e.evalSuccesses, e.evalWarnings, e.evalFailures, len(evalLog.Evaluations))

	e.EndTime = time.Now().String()

	if e.changeManager != nil {
		e.changeManager.RevertAll()
		if e.CorruptedState {
			return CORRUPTION_FOUND("ev40")
		}
	}

	switch e.Result {
	case gemara.Passed:
		e.config.Logger.Info(output)
	case gemara.NotRun:
		e.config.Logger.Trace(output)
	default:
		e.config.Logger.Error(output)
	}
	return nil
}

// GetAssessmentRequirements retrieves all assessment requirements from the catalog.
func (e *EvaluationSuite) GetAssessmentRequirements() (map[string]*gemara.AssessmentRequirement, error) {
	requirements := make(map[string]*gemara.AssessmentRequirement)
	for _, control := range e.catalog.Controls {
		for _, requirement := range control.AssessmentRequirements {
			requirements[requirement.Id] = &requirement
		}
	}

	if len(requirements) == 0 {
		return nil, NO_ASSESSMENT_REQS_PROVIDED("ev50")
	}

	return requirements, nil
}

func (e *EvaluationSuite) setupEvalLog(steps map[string][]gemara.AssessmentStep) (evalLog gemara.EvaluationLog, err error) {
	if len(steps) == 0 {
		return evalLog, NO_ASSESSMENT_STEPS_PROVIDED("sel10")
	}

	// crash if reaching the end without a requirement
	// use errMod to add level of detail
	var controlsFound bool
	var reqsFound bool

	if len(e.catalog.Families) == 0 {
		return evalLog, EVAL_SUITE_CRASHED("sel20")
	}

	for _, control := range e.catalog.Controls {
		controlsFound = true
		if len(control.AssessmentRequirements) == 0 {
			continue
		}
		reqsFound = true

		// Create ControlEvaluation first
		evaluation := &gemara.ControlEvaluation{
			Name: control.Title,
			Control: gemara.EntryMapping{
				ReferenceId: e.CatalogId,
				EntryId:     control.Id,
			},
		}

		for _, requirement := range control.AssessmentRequirements {
			// Use AddAssessment instead of manual struct creation
			assessment := evaluation.AddAssessment(
				requirement.Id,            // requirementId
				control.Objective,         // description
				requirement.Applicability, // applicability
				steps[requirement.Id],     // steps
			)

			// Handle case where no steps were found
			if _, ok := steps[requirement.Id]; !ok {
				assessment.Result = gemara.Unknown
				if e.config != nil {
					msg := fmt.Sprintf("requirement: %s, control %s+sel30", requirement.Id, control.Id)
					err := NO_ASSESSMENT_STEPS_PROVIDED(msg)
					e.config.Logger.Debug(err.Error())
				}
			}
		}
		evalLog.Evaluations = append(evalLog.Evaluations, evaluation)
	}

	if !controlsFound {
		return evalLog, EVAL_SUITE_CRASHED("sel40")
	}
	if !reqsFound {
		return evalLog, EVAL_SUITE_CRASHED("sel50")
	}

	return evalLog, nil
}
