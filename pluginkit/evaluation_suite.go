package pluginkit

import (
	"fmt"
	"strings"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/config"
)

// TestSet is a function type that returns a control evaluation result.
type TestSet func() (result gemara.ControlEvaluation)

// EvaluationSuite contains the results of all EvaluationLog executions.
// Exported fields will be used in the final YAML or JSON output documents.
type EvaluationSuite struct {
	Name   string        `json:"name" yaml:"name"`     // Name is the name of the suite
	Result gemara.Result `json:"result" yaml:"result"` // Result is Passed if all evaluations in the suite passed

	CatalogId string `json:"catalog-id" yaml:"catalog-id"` // CatalogId associates this suite with a catalog
	StartTime string `json:"start-time" yaml:"start-time"` // StartTime is the time the plugin started
	EndTime   string `json:"end-time" yaml:"end-time"`     // EndTime is the time the plugin ended

	CorruptedState bool `json:"corrupted-state" yaml:"corrupted-state"` // CorruptedState is true if any testSet failed to revert at the end of the evaluation

	EvaluationLog gemara.EvaluationLog `json:"control-evaluations" yaml:"control-evaluations"` // EvaluationLog is a slice of evaluations to be executed

	config *config.Config // config is the global configuration

	payload       interface{}                        // payload is the data to be evaluated
	loader        DataLoader                         // loader is the function to load the payload
	changeManager *ChangeManager                     // changes is a list of changes made during the evaluation
	catalog       *gemara.ControlCatalog             // The Catalog this evaluation suite references
	steps         map[string][]gemara.AssessmentStep // steps is a map of control IDs to their assessment steps
	stepNames     map[string][]string                // step names captured at registration, parallel to steps; nil falls back to symbol lookup

	evalSuccesses int // successes is the number of successful evaluations
	evalFailures  int // failures is the number of failed evaluations
	evalWarnings  int // warnings is the number of evaluations that need review

	durationNs  int64        // benchmark mode only
	stepTimings []StepTiming // benchmark mode only
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
	started := time.Now()
	e.StartTime = started.UTC().Format(time.RFC3339Nano)
	if e.config.Benchmark {
		// duration from the monotonic clock, never timestamp subtraction
		defer func() { e.durationNs = time.Since(started).Nanoseconds() }()
	}

	e.config.Logger.Trace("Starting evaluation", "name", e.Name, "time", e.StartTime)

	for _, evaluation := range e.EvaluationLog.Evaluations {
		evaluation.Evaluate(e.payload, e.config.Policy.Applicability)

		// Make sure the evaluation result is updated based on the complete assessment results
		e.Result = gemara.UpdateAggregateResult(e.Result, evaluation.Result)

		// Log each assessment result as a separate line
		for _, assessment := range evaluation.AssessmentLogs {
			message := fmt.Sprintf("%s: %s", assessment.Requirement.EntryId, singleLine(assessment.Message))
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

	e.restoreSteps()

	e.EndTime = time.Now().UTC().Format(time.RFC3339Nano)

	if e.changeManager != nil {
		e.changeManager.RevertAll()
		// The ChangeManager tracks corruption per change; sync it onto the suite
		// so the written results (and the gemara EvaluationLog) can report it.
		e.CorruptedState = e.changeManager.CorruptedState
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

// restoreSteps restores benchmark-wrapped steps back to the original for stack tracing
func (e *EvaluationSuite) restoreSteps() {
	if e.config == nil || !e.config.Benchmark {
		return
	}
	for _, evaluation := range e.EvaluationLog.Evaluations {
		for _, assessment := range evaluation.AssessmentLogs {
			assessment.Steps = e.steps[assessment.Requirement.EntryId]
		}
	}
}

// stepName returns the name captured at registration for the step at index i of
// requirementId, falling back to symbol lookup when the plugin registered
// untyped steps. Symbol lookup is only meaningful when the registered value is
// the plugin's own function; a plugin-side adapter closure resolves to the
// adapter, identically for every step it wraps.
func (e *EvaluationSuite) stepName(requirementId string, i int, step gemara.AssessmentStep) string {
	if names, ok := e.stepNames[requirementId]; ok && i < len(names) && names[i] != "" {
		return names[i]
	}
	return step.String()
}

// timedSteps wraps each executed step in a closure that records its duration, name, and result.
func (e *EvaluationSuite) timedSteps(controlId, requirementId string, steps []gemara.AssessmentStep) []gemara.AssessmentStep {
	if len(steps) == 0 {
		return steps
	}
	timed := make([]gemara.AssessmentStep, len(steps))
	for i, step := range steps {
		name := e.stepName(requirementId, i, step)
		timed[i] = func(payload interface{}) (gemara.Result, string, gemara.ConfidenceLevel) {
			start := time.Now()
			result, message, confidence := step(payload)
			e.stepTimings = append(e.stepTimings, StepTiming{
				ControlId:     controlId,
				RequirementId: requirementId,
				StepIndex:     i,
				Step:          name,
				Result:        result.String(),
				DurationNs:    time.Since(start).Nanoseconds(),
			})
			return result, message, confidence
		}
	}
	return timed
}

// singleLine collapses a multi-line assessment message into one line so the
// log stays one entry per assessment; the written results keep the message verbatim.
func singleLine(message string) string {
	return strings.Join(strings.Fields(message), " ")
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

	if len(e.catalog.Controls) == 0 {
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
			// benchmark mode times each step; later on restoreSteps will unwrap this before serialization
			reqSteps := steps[requirement.Id]
			if e.config != nil && e.config.Benchmark {
				reqSteps = e.timedSteps(control.Id, requirement.Id, reqSteps)
			}

			// Use AddAssessment instead of manual struct creation
			assessment := evaluation.AddAssessment(
				requirement.Id,            // requirementId
				control.Objective,         // description
				requirement.Applicability, // applicability
				reqSteps,                  // steps
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
