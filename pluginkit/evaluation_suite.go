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
	catalog       *gemara.ControlCatalog             // The Catalog this evaluation suite evaluates: source plus resolved imports
	source        *gemara.ControlCatalog             // The registered catalog, untouched; Mobilize rebuilds catalog from it
	steps         map[string][]gemara.AssessmentStep // steps is a map of control IDs to their assessment steps
	invalid       map[string]string                  // invalid maps requirement IDs the catalog leaves unjudgeable to the reason

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
		// gemara cannot fail a requirement whose catalog entry is malformed: it
		// marks one with no applicability NotApplicable and one with no objective
		// Unknown or NotApplicable. Fail them here, then recompute the control so
		// the result gemara rolled up does not hide the failure.
		if e.failInvalid(evaluation) {
			recomputeControlResult(evaluation)
		}

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

// failInvalid marks each assessment of a malformed requirement Failed with the
// reason it cannot be judged, and reports whether any changed.
func (e *EvaluationSuite) failInvalid(evaluation *gemara.ControlEvaluation) bool {
	changed := false
	for _, assessment := range evaluation.AssessmentLogs {
		msg, ok := e.invalid[assessment.Requirement.EntryId]
		if !ok {
			continue
		}
		assessment.Result = gemara.Failed
		assessment.Message = msg
		changed = true
	}
	return changed
}

// recomputeControlResult rebuilds the control's Result and Message from its
// assessments the same way gemara's ControlEvaluation.Evaluate rolls them up.
func recomputeControlResult(evaluation *gemara.ControlEvaluation) {
	evaluation.Result = gemara.NotRun
	evaluation.Message = ""
	for _, assessment := range evaluation.AssessmentLogs {
		aggregate := gemara.UpdateAggregateResult(evaluation.Result, assessment.Result)
		if aggregate != evaluation.Result {
			evaluation.Message = assessment.Message
		}
		evaluation.Result = aggregate
	}
}

// invalidRequirementReason returns why a requirement cannot be judged, or ""
// if it can. An empty applicability cannot be judged in or out of scope, and a
// control with no objective gives its requirements nothing to assess against.
func invalidRequirementReason(control gemara.Control, requirement gemara.AssessmentRequirement) string {
	var defects []string
	if len(requirement.Applicability) == 0 {
		defects = append(defects, "requirement has no applicability")
	}
	if control.Objective == "" {
		defects = append(defects, "control has no objective")
	}
	if len(defects) == 0 {
		return ""
	}
	return fmt.Sprintf("cannot evaluate requirement %q of control %q: %s", requirement.Id, control.Id, strings.Join(defects, "; "))
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

// timedSteps wraps each executed step in a closure that records its duration, name, and result.
func (e *EvaluationSuite) timedSteps(controlId, requirementId string, steps []gemara.AssessmentStep) []gemara.AssessmentStep {
	if len(steps) == 0 {
		return steps
	}
	timed := make([]gemara.AssessmentStep, len(steps))
	for i, step := range steps {
		name := step.String()
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
	e.invalid = make(map[string]string)

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

			// NewAssessment rather than AddAssessment: AddAssessment also marks the
			// control Failed when gemara's precheck rejects the assessment, and
			// whether that survives ControlEvaluation.Evaluate depends on the gemara
			// version. The SDK validates requirements itself (invalidRequirementReason),
			// and Run repeats the precheck, so the precheck error adds nothing here.
			assessment, _ := gemara.NewAssessment(
				requirement.Id,            // requirementId
				control.Objective,         // description
				requirement.Applicability, // applicability
				reqSteps,                  // steps
			)
			assessment.Requirement.ReferenceId = e.CatalogId
			evaluation.AssessmentLogs = append(evaluation.AssessmentLogs, assessment)

			if reason := invalidRequirementReason(control, requirement); reason != "" {
				e.invalid[requirement.Id] = reason
			}

			// Run's precheck reports an applicable requirement with no steps as
			// Unknown; an inapplicable one is NotApplicable.
			if _, ok := steps[requirement.Id]; !ok {
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
