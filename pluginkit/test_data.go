package pluginkit

import (
	"fmt"
	"os"

	"github.com/ossf/gemara/layer2"
	"github.com/ossf/gemara/layer4"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
)

type testingData struct {
	testName               string
	evals                  []*layer4.ControlEvaluation // Keep for backward compatibility with other tests
	steps                  map[string][]layer4.AssessmentStep
	expectedEvalSuiteError error
	expectedResult         layer4.Result
}

var testCatalog = &layer2.Catalog{
	ControlFamilies: []layer2.ControlFamily{},
}

func getTestCatalog() (*layer2.Catalog, error) {
	if len(testCatalog.ControlFamilies) > 0 {
		return testCatalog, nil
	}
	catalog := &layer2.Catalog{}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get working directory when retrieving catalog: %w", err)
	}
	file1 := fmt.Sprintf("file://%s/catalog-test-data/metadata.yaml", pwd)
	file2 := fmt.Sprintf("file://%s/catalog-test-data/controls.yaml", pwd)
	err = catalog.LoadFiles([]string{file1, file2})
	if err != nil {
		return nil, err
	}
	return catalog, nil
}

// getEmptyTestCatalog returns an empty catalog for testing error conditions
func getEmptyTestCatalog() *layer2.Catalog {
	return &layer2.Catalog{
		ControlFamilies: []layer2.ControlFamily{},
	}
}

// getTestCatalogWithNoRequirements returns a catalog with controls but no assessment requirements
func getTestCatalogWithNoRequirements() *layer2.Catalog {
	return &layer2.Catalog{
		ControlFamilies: []layer2.ControlFamily{
			{
				Id: "test-family",
				Controls: []layer2.Control{
					{
						Id:                     "test-control",
						Title:                  "Test Control",
						Objective:              "Test objective",
						AssessmentRequirements: []layer2.AssessmentRequirement{}, // Empty requirements
					},
				},
			},
		},
	}
}

func passingEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		Control: layer4.Mapping{
			EntryId: "good-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-good",
		"this assessment should work fine",
		requestedApplicability,
		[]layer4.AssessmentStep{
			step_Pass,
		},
	)
	return
}

func failingEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		Control: layer4.Mapping{
			EntryId: "bad-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-bad",
		"this assessment should fail",
		requestedApplicability,
		[]layer4.AssessmentStep{
			step_Pass,
			step_Fail,
		},
	)
	return
}

func needsReviewEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		Control: layer4.Mapping{
			EntryId: "needs-review-evaluation",
		},
	}

	evaluation.AddAssessment(
		"assessment-review",
		"this assessment should need review",
		requestedApplicability,
		[]layer4.AssessmentStep{
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

func step_Pass(data interface{}) (result layer4.Result, message string) {
	return layer4.Passed, "This step always passes"
}

func step_Fail(_ interface{}) (result layer4.Result, message string) {
	return layer4.Failed, "This step always fails"
}

func step_NeedsReview(_ interface{}) (result layer4.Result, message string) {
	return layer4.NeedsReview, "This step always needs review"
}

// Helper function to create simple passing steps map
func createPassingStepsMap() map[string][]layer4.AssessmentStep {
	return map[string][]layer4.AssessmentStep{
		"CCC.Core.C01.TR01": {step_Pass},
	}
}

// Helper function to get all requirement IDs from the test catalog
func getAllRequirementIds() ([]string, error) {
	catalog, err := getTestCatalog()
	if err != nil {
		return nil, err
	}

	var requirementIds []string
	for _, family := range catalog.ControlFamilies {
		for _, control := range family.Controls {
			for _, requirement := range control.AssessmentRequirements {
				requirementIds = append(requirementIds, requirement.Id)
			}
		}
	}
	return requirementIds, nil
}

// Helper function to convert evaluations to steps map for testing
// This is a simplified conversion for backward compatibility with existing tests
func convertEvalsToStepsMap(evals []*layer4.ControlEvaluation) map[string][]layer4.AssessmentStep {
	stepsMap := make(map[string][]layer4.AssessmentStep)

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
	var primarySteps []layer4.AssessmentStep
	if len(evals) > 0 {
		switch evals[0].Control.EntryId {
		case "good-evaluation":
			primarySteps = []layer4.AssessmentStep{step_Pass}
		case "bad-evaluation":
			primarySteps = []layer4.AssessmentStep{step_Pass, step_Fail}
		case "needs-review-evaluation":
			primarySteps = []layer4.AssessmentStep{step_NeedsReview}
		default:
			primarySteps = []layer4.AssessmentStep{step_Pass}
		}
	} else {
		primarySteps = []layer4.AssessmentStep{step_Pass}
	}

	// Apply the pattern based on the number of evaluations
	for i, requirementId := range requirementIds {
		if len(evals) > 1 && i < len(evals) {
			// For multiple evaluations, use each evaluation's specific pattern
			eval := evals[i]
			switch eval.Control.EntryId {
			case "good-evaluation":
				stepsMap[requirementId] = []layer4.AssessmentStep{step_Pass}
			case "bad-evaluation":
				stepsMap[requirementId] = []layer4.AssessmentStep{step_Pass, step_Fail}
			case "needs-review-evaluation":
				stepsMap[requirementId] = []layer4.AssessmentStep{step_NeedsReview}
			default:
				stepsMap[requirementId] = []layer4.AssessmentStep{step_Pass}
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
			expectedResult: layer4.Passed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*layer4.ControlEvaluation{
				passingEvaluation(),
			}),
		},
		{
			testName:       "Empty Steps Map",
			expectedResult: layer4.NotRun,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
			steps:                  map[string][]layer4.AssessmentStep{},
			expectedEvalSuiteError: NO_ASSESSMENT_STEPS_PROVIDED("sel10"),
		},
		{
			testName:       "Nil Steps Map",
			expectedResult: layer4.NotRun,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
			},
			steps:                  nil,
			expectedEvalSuiteError: NO_ASSESSMENT_STEPS_PROVIDED("sel10"),
		},
		{
			testName:       "Mixed Evaluation Results",
			expectedResult: layer4.Failed,
			evals: []*layer4.ControlEvaluation{
				passingEvaluation(),
				failingEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*layer4.ControlEvaluation{
				passingEvaluation(),
				failingEvaluation(),
			}),
		},
		{
			testName:       "Needs Review Evaluation",
			expectedResult: layer4.NeedsReview,
			evals: []*layer4.ControlEvaluation{
				needsReviewEvaluation(),
			},
			steps: convertEvalsToStepsMap([]*layer4.ControlEvaluation{
				needsReviewEvaluation(),
			}),
		},
		{
			testName:       "Empty Evaluations List",
			expectedResult: layer4.NotRun,
			evals:          []*layer4.ControlEvaluation{},
			steps:          convertEvalsToStepsMap([]*layer4.ControlEvaluation{}),
		},
		{
			testName:       "Nil Evaluations List",
			expectedResult: layer4.NotRun,
			evals:          nil,
			steps:          convertEvalsToStepsMap(nil),
		},
	}
}
