package pluginkit

import (
	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
)

var (
	testingApplicabilityString = "test"

	testingConfig = &config.Config{
		ServiceName: "test-service",
		Policy: config.Policy{
			Applicability: testingApplicabilityString,
		},
		Logger: hclog.NewNullLogger(),
	}

	testingCatalogEvaluations = map[string]EvaluationSuite{
		"catalog1": {
			Name: "corrupted-failed",
			Control_Evaluations: []layer4.ControlEvaluation{
				{
					Message:         "test",
					Result:          layer4.Failed,
					Corrupted_State: true,
					Assessments: []layer4.Assessment{
						{
							Result:      layer4.Failed,
							Description: "test",
							Applicability: []string{
								testingApplicabilityString,
							},
						},
					},
				},
			},
		},
	}

	testData = []struct {
		testName                     string
		data                         map[string]EvaluationSuite
		expectedEvaluationSuiteError error
	}{
		{
			testName:                     "Corrupted State Evaluation",
			data:                         testingCatalogEvaluations,
			expectedEvaluationSuiteError: CORRUPTION_FOUND(),
		},
	}
)
