package pluginkit

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
)

var (
	testingApplicabilityString = "test"
	testingCatalogName         = "catalog1"
	testingServiceName         = "test-service"
	testingEvaluationName      = fmt.Sprintf("%s_%s", testingServiceName, testingCatalogName)

	testingConfig = &config.Config{
		ServiceName: testingServiceName,
		Policy: config.Policy{
			Applicability:   testingApplicabilityString,
			ControlCatalogs: []string{testingCatalogName},
		},
		Logger: hclog.NewNullLogger(),
	}

	testingCatalogEvaluations = map[string]EvaluationSuite{
		testingCatalogName: {
			Name: "corrupted-failed",
			Control_Evaluations: []*layer4.ControlEvaluation{
				{
					Name:            testingCatalogName,
					Message:         "test",
					Result:          layer4.Failed,
					Corrupted_State: true,
					Assessments: []*layer4.Assessment{
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
