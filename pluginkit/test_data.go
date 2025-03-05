package pluginkit

import (
	"fmt"

	"github.com/revanite-io/sci/pkg/layer4"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
)

type testingData struct {
	testName               string
	serviceName            string
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
		Control_Id: "good-evaluation",
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
		func() (interface{}, error) {
			return nil, nil
		},
		func() error {
			return nil
		},
	)
	return
}

func failingEvaluation() (evaluation *layer4.ControlEvaluation) {
	evaluation = &layer4.ControlEvaluation{
		Control_Id: "bad-evaluation",
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
		Control_Id: "needs-review-evaluation",
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
		Control_Id: "corrupted-evaluation",
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
		func() (interface{}, error) {
			return nil, fmt.Errorf("corrupted")
		},
		func() error {
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

func step_checkPayload(payloadData interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	payload, ok := payloadData.(*PayloadTypeExample)
	if !ok {
		return layer4.Unknown, fmt.Sprintf("Malformed assessment: expected payload type %T, got %T (%v)", &PayloadTypeExample{}, payloadData, payloadData)
	}
	if payload.CustomPayloadField {
		return layer4.Passed, "Multi-factor authentication is configured"
	}
	return layer4.Unknown, "Not implemented"
}

func step_Pass(_ interface{}, changes map[string]*layer4.Change) (result layer4.Result, message string) {
	if changes != nil && changes["good-change"] != nil {
		changes["good-change"].Apply()
	}
	return layer4.Passed, "This step always passes"
}

func step_Fail(_ interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	return layer4.Failed, "This step always fails"
}

func step_NeedsReview(_ interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	return layer4.NeedsReview, "This step always needs review"
}

func step_Unknown(_ interface{}, _ map[string]*layer4.Change) (result layer4.Result, message string) {
	return layer4.Unknown, "This step always returns unknown"
}

func step_Corrupted(_ interface{}, changes map[string]*layer4.Change) (result layer4.Result, message string) {
	changes["corrupted-change"].Apply()
	return layer4.Unknown, "This step always returns unknown and applies a corrupted change"
}
