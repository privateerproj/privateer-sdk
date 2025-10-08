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
	steps                  []*layer4.AssessmentStep
	expectedEvalSuiteError error
	expectedCorruption     bool
	expectedResult         layer4.Result
}

var testPayload = interface{}(PayloadTypeExample{CustomPayloadField: true})

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

func examplePayload(_ *config.Config) (interface{}, error) {
	return testPayload, nil
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

var requestedApplicability = []string{"valid-applicability-1", "valid-applicability-2"}
var requestedCatalogs = []string{"catalog1", "catalog2", "catalog3"}

func setBasicConfig() *config.Config {
	viper.Set("service", "test-service")
	viper.Set("policy.applicability", requestedApplicability)
	viper.Set("policy.catalogs", requestedCatalogs)
	c := config.NewConfig(nil)
	return &c
}

func setLimitedConfig() *config.Config {
	viper.Set("service", "test-service")
	viper.Set("services.test-service.policy.applicability", requestedApplicability[0])
	viper.Set("policy.catalogs", requestedCatalogs[0])
	c := config.NewConfig(nil)
	return &c
}

type PayloadTypeExample struct {
	CustomPayloadField bool
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

var mobilizeTestData = []testingData{
	{
		testName:       "Pass Evaluation",
		expectedResult: layer4.Passed,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
		},
	},
	{
		testName:       "Fail Evaluation",
		expectedResult: layer4.Failed,
		evals: []*layer4.ControlEvaluation{
			failingEvaluation(),
		},
	},
	{
		testName:       "Needs Review Evaluation",
		expectedResult: layer4.NeedsReview,
		evals: []*layer4.ControlEvaluation{
			needsReviewEvaluation(),
		},
	},
	{
		testName:       "Pass Pass Pass",
		expectedResult: layer4.Passed,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			passingEvaluation(),
			passingEvaluation(),
		},
	},
	{
		testName:       "Pass Then Fail",
		expectedResult: layer4.Failed,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			failingEvaluation(),
		},
	},
	{
		testName:       "Pass Then Needs Review",
		expectedResult: layer4.NeedsReview,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			needsReviewEvaluation(),
		},
	},
	{
		testName:       "Needs Review Then Pass",
		expectedResult: layer4.NeedsReview,
		evals: []*layer4.ControlEvaluation{
			needsReviewEvaluation(),
			passingEvaluation(),
		},
	},
	{
		testName:       "Needs Review Then Fail",
		expectedResult: layer4.Failed,
		evals: []*layer4.ControlEvaluation{
			needsReviewEvaluation(),
			failingEvaluation(),
		},
	},
}
