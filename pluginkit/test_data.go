package pluginkit

import (
	"fmt"

	"github.com/ossf/gemara/layer4"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
)

type testingData struct {
	testName               string
	evals                  []*layer4.ControlEvaluation
	expectedEvalSuiteError error
	expectedCorruption     bool
	expectedResult         layer4.Result
}

var testPayload = interface{}(PayloadTypeExample{CustomPayloadField: true})

func examplePayload(_ *config.Config) (interface{}, error) {
	return testPayload, nil
}

func passingEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		ControlID: "good-evaluation",
	}

	assessment := evaluation.AddAssessment(
		"assessment-good",
		"this assessment should work fine",
		requestedApplicability,
		[]layer4.AssessmentStep{
			step_Pass,
		},
	)
	assessment.NewChange(
		"good-change",
		"fake-target-name",
		"this change doesn't do anything",
		nil,
		func(interface{}) (interface{}, error) {
			return nil, nil
		},
		func(interface{}) error {
			return nil
		},
	)
	return
}

func failingEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		ControlID: "bad-evaluation",
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
		ControlID: "needs-review-evaluation",
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

func corruptedEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		ControlID: "corrupted-evaluation",
	}

	assessment := evaluation.AddAssessment(
		"assessment-corrupted",
		"this assessment should be corrupted",
		requestedApplicability,
		[]layer4.AssessmentStep{
			step_Corrupted,
		},
	)
	assessment.NewChange(
		"corrupted-change",
		"fake-target-name",
		"this change doesn't do anything",
		nil,
		func(interface{}) (interface{}, error) {
			return nil, nil
		},
		func(interface{}) error {
			return fmt.Errorf("corrupted")
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

func step_Pass(data interface{}, changes map[string]*layer4.Change) (result layer4.Result, message string) {
	if changes != nil && changes["good-change"] != nil {
		changes["good-change"].Apply("target_name", "target_object", data)
	}
	return layer4.Passed, "This step always passes"
}

func step_Fail(_ interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	return layer4.Failed, "This step always fails"
}

func step_NeedsReview(_ interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	return layer4.NeedsReview, "This step always needs review"
}

func step_Corrupted(data interface{}, changes map[string]*layer4.Change) (result layer4.Result, message string) {
	changes["corrupted-change"].Apply("target_name", "target_object", data)
	return layer4.Unknown, "This step always returns unknown and applies a corrupted change"
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
		testName:           "Corrupted Evaluation",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			corruptedEvaluation(),
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
		testName:           "Pass Then Corrupted",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			corruptedEvaluation(),
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
	{
		testName:           "Corrupt Pass Pass",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			corruptedEvaluation(),
			passingEvaluation(),
			passingEvaluation(),
		},
	},
	{
		testName:           "Pass Corrupt Pass",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			corruptedEvaluation(),
			passingEvaluation(),
		},
	},
	{
		testName:           "Pass Pass Corrupt",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			passingEvaluation(),
			passingEvaluation(),
			corruptedEvaluation(),
		},
	},
	{
		testName:           "Corrupt Corrupt Pass",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			corruptedEvaluation(),
			corruptedEvaluation(),
			passingEvaluation(),
		},
	},
	{
		testName:           "Corrupt Corrupt Corrupt",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			corruptedEvaluation(),
			corruptedEvaluation(),
			corruptedEvaluation(),
		},
	},
	{
		testName:           "Corrupt then Needs Review",
		expectedResult:     layer4.Unknown,
		expectedCorruption: true,
		evals: []*layer4.ControlEvaluation{
			corruptedEvaluation(),
			needsReviewEvaluation(),
		},
	},
}
