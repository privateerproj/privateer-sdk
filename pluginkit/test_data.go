package pluginkit

import (
	"github.com/gemaraproj/go-gemara"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
)

type testingData struct {
	testName               string
	evals                  []*gemara.ControlEvaluation // Keep for backward compatibility with other tests
	steps                  map[string][]gemara.AssessmentStep
	expectedEvalSuiteError error
	expectedResult         gemara.Result
}

// getEmptyTestCatalog returns an empty catalog for testing error conditions
func getEmptyTestCatalog() *gemara.ControlCatalog {
	return &gemara.ControlCatalog{
		Controls: []gemara.Control{},
	}
}

// getTestCatalogWithNoRequirements returns a catalog with controls but no assessment requirements
func getTestCatalogWithNoRequirements() *gemara.ControlCatalog {
	return &gemara.ControlCatalog{
		Controls: []gemara.Control{
			{
				Id:                     "test-control",
				Title:                  "Test Control",
				Objective:              "Test objective",
				AssessmentRequirements: []gemara.AssessmentRequirement{}, // Empty requirements
			},
		},
	}
}

// getTestCatalogWithRequirements returns a catalog with controls and assessment requirements for tests that need a valid catalog.
func getTestCatalogWithRequirements() *gemara.ControlCatalog {
	return &gemara.ControlCatalog{
		Metadata: gemara.Metadata{Id: "CCC.ObjStor"},
		Controls: []gemara.Control{
			{
				Id:        "CCC.Core.C01",
				Title:     "Encrypt Data for Transmission",
				Objective: "Ensure that all communications are encrypted in transit.",
				AssessmentRequirements: []gemara.AssessmentRequirement{
					{
						Id:           "CCC.Core.C01.TR01",
						Text:         "When a port is exposed for non-SSH network traffic, all traffic MUST include a TLS handshake.",
						Applicability: requestedApplicability,
					},
				},
			},
		},
	}
}

func passingEvaluation() (evaluation *gemara.ControlEvaluation) {
	evaluation = &gemara.ControlEvaluation{
		Control: gemara.EntryMapping{
			EntryId: "good-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-good",
		"this assessment should work fine",
		requestedApplicability,
		[]gemara.AssessmentStep{
			step_Pass,
		},
	)
	return
}

func failingEvaluation() (evaluation *gemara.ControlEvaluation) {
	evaluation = &gemara.ControlEvaluation{
		Control: gemara.EntryMapping{
			EntryId: "bad-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-bad",
		"this assessment should fail",
		requestedApplicability,
		[]gemara.AssessmentStep{
			step_Pass,
			step_Fail,
		},
	)
	return
}

func needsReviewEvaluation() (evaluation *gemara.ControlEvaluation) {
	evaluation = &gemara.ControlEvaluation{
		Control: gemara.EntryMapping{
			EntryId: "needs-review-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-review",
		"this assessment should need review",
		requestedApplicability,
		[]gemara.AssessmentStep{
			step_NeedsReview,
		},
	)
	return
}

var requestedApplicability = []string{"tlp-green", "tlp-amber"}
var requestedCatalogs = []string{"catalog1", "catalog2", "catalog3"}

func setBasicConfig() *config.Config {
	viper.Set("service", "test-service")
	viper.Set("policy.applicability", requestedApplicability)
	viper.Set("policy.catalogs", requestedCatalogs)
	c := config.NewConfig(nil)
	return &c
}

func step_Pass(data interface{}) (result gemara.Result, message string, confidence gemara.ConfidenceLevel) {
	return gemara.Passed, "This step always passes", gemara.High
}

func step_Fail(_ interface{}) (result gemara.Result, message string, confidence gemara.ConfidenceLevel) {
	return gemara.Failed, "This step always fails", gemara.Low
}

func step_NeedsReview(_ interface{}) (result gemara.Result, message string, confidence gemara.ConfidenceLevel) {
	return gemara.NeedsReview, "This step always needs review", gemara.Medium
}

// Helper function to create simple passing steps map
func createPassingStepsMap() map[string][]gemara.AssessmentStep {
	return map[string][]gemara.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Pass},
	}
}

// Helper function to get all requirement IDs from the test catalog.
// Uses getTestCatalogWithRequirements() so steps built for tests match the fixture catalog.
func getAllRequirementIds() ([]string, error) {
	catalog := getTestCatalogWithRequirements()

	var requirementIds []string
	for _, control := range catalog.Controls {
		for _, requirement := range control.AssessmentRequirements {
			requirementIds = append(requirementIds, requirement.Id)
		}
	}
	return requirementIds, nil
}

// Helper function to convert evaluations to steps map for testing
// This is a simplified conversion for backward compatibility with existing tests
func convertEvalsToStepsMap(evals []*gemara.ControlEvaluation) map[string][]gemara.AssessmentStep {
	stepsMap := make(map[string][]gemara.AssessmentStep)

	// Handle empty evaluations
	if len(evals) == 0 {
		// Return empty map for empty evaluations
		return stepsMap
	}

	// Get all requirement IDs from the actual test catalog
	requirementIds, err := getAllRequirementIds()
	if err != nil {
		// Fallback to a basic set if catalog loading fails
		requirementIds = []string{"CCC.Core.C01.TR01"}
	}

	// If no requirement IDs found, return empty map
	if len(requirementIds) == 0 {
		return stepsMap
	}

	// Determine the steps to use based on the evaluation type
	var primarySteps []gemara.AssessmentStep
	if len(evals) > 0 {
		switch evals[0].Control.EntryId {
		case "good-evaluation":
			primarySteps = []gemara.AssessmentStep{step_Pass}
		case "bad-evaluation":
			primarySteps = []gemara.AssessmentStep{step_Pass, step_Fail}
		case "needs-review-evaluation":
			primarySteps = []gemara.AssessmentStep{step_NeedsReview}
		default:
			primarySteps = []gemara.AssessmentStep{step_Pass}
		}
	} else {
		primarySteps = []gemara.AssessmentStep{step_Pass}
	}

	// Apply the pattern based on the number of evaluations
	for i, requirementId := range requirementIds {
		if len(evals) > 1 && i < len(evals) {
			// For multiple evaluations, use each evaluation's specific pattern
			eval := evals[i]
			switch eval.Control.EntryId {
			case "good-evaluation":
				stepsMap[requirementId] = []gemara.AssessmentStep{step_Pass}
			case "bad-evaluation":
				stepsMap[requirementId] = []gemara.AssessmentStep{step_Pass, step_Fail}
			case "needs-review-evaluation":
				stepsMap[requirementId] = []gemara.AssessmentStep{step_NeedsReview}
			default:
				stepsMap[requirementId] = []gemara.AssessmentStep{step_Pass}
			}
		} else {
			// For single evaluation or remaining requirements, use the primary pattern
			stepsMap[requirementId] = primarySteps
		}
	}

	return stepsMap
}

// Test data for the main TestEvaluate function
func getTestEvaluateData() []testingData {
	return []testingData{
		{
			testName:       "Good Evaluation",
			expectedResult: gemara.Passed,
			evals: []*gemara.ControlEvaluation{
				passingEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*gemara.ControlEvaluation{
				passingEvaluation(),
			}),
		},
		{
			testName:       "Empty Steps Map",
			expectedResult: gemara.NotRun,
			evals: []*gemara.ControlEvaluation{
				passingEvaluation(),
			},
			steps:                  map[string][]gemara.AssessmentStep{},
			expectedEvalSuiteError: NO_ASSESSMENT_STEPS_PROVIDED("sel10"),
		},
		{
			testName:       "Nil Steps Map",
			expectedResult: gemara.NotRun,
			evals: []*gemara.ControlEvaluation{
				passingEvaluation(),
			},
			steps:                  nil,
			expectedEvalSuiteError: NO_ASSESSMENT_STEPS_PROVIDED("sel10"),
		},
		{
			testName:       "Mixed Evaluation Results",
			expectedResult: gemara.Failed,
			evals: []*gemara.ControlEvaluation{
				passingEvaluation(),
				failingEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*gemara.ControlEvaluation{
				passingEvaluation(),
				failingEvaluation(),
			}),
		},
		{
			testName:       "Needs Review Evaluation",
			expectedResult: gemara.NeedsReview,
			evals: []*gemara.ControlEvaluation{
				needsReviewEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*gemara.ControlEvaluation{
				needsReviewEvaluation(),
			}),
		},
		{
			testName:       "Empty Evaluations List",
			expectedResult: gemara.NotRun,
			evals:          []*gemara.ControlEvaluation{},
			steps:          convertEvalsToStepsMap([]*gemara.ControlEvaluation{}),
		},
		{
			testName:       "Nil Evaluations List",
			expectedResult: gemara.NotRun,
			evals:          nil,
			steps:          convertEvalsToStepsMap(nil),
		},
	}
}
