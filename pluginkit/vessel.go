package pluginkit

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
)

// The vessel gets the armory in position to execute the testSets specified in the testSuites
type Vessel struct {
	ServiceName        string
	PluginName         string
	RequiredVars       []string
	Armory             *Armory
	ControlEvaluations []layer4.ControlEvaluation
	Initializer        func(*config.Config) error
	config             *config.Config
	logger             hclog.Logger
	executedTestSets   *[]string
}

func NewVessel(
	name string,
	armory *Armory,
	initializer func(*config.Config) error,
	requiredVars []string) Vessel {

	return Vessel{
		PluginName:   name,
		Armory:       armory,
		Initializer:  initializer,
		RequiredVars: requiredVars,
	}
}

// StockArmory sets up the armory for the vessel to use
func (v *Vessel) StockArmory() error {
	if v.Armory == nil {
		return errors.New("vessel's Armory field cannot be nil")
	}
	if v.logger == nil {
		if v.config == nil {
			config := config.NewConfig(v.RequiredVars)
			v.config = &config
		}
	}
	if v.config.Error != nil {
		return v.config.Error
	}

	v.Armory.Config = v.config
	v.Armory.Logger = v.config.Logger
	v.Armory.ServiceTarget = v.ServiceName

	v.logger = v.config.Logger
	v.ServiceName = v.config.ServiceName

	if v.PluginName == "" || v.ServiceName == "" {
		return fmt.Errorf("expected service and plugin names to be set. ServiceName='%s' PluginName='%s'", v.ServiceName, v.PluginName)
	}
	if v.Armory == nil {
		return fmt.Errorf("no armory was stocked for the plugin '%s'", v.PluginName)
	}
	if v.Armory.EvaluationSuites == nil {
		return fmt.Errorf("no testSuites provided for the service")
	}

	return nil
}

// Mobilize executes the testSets specified in the testSuites
func (v *Vessel) Mobilize() (err error) {
	err = v.StockArmory()
	if err != nil {
		return
	}
	if v.config == nil {
		err = fmt.Errorf("failed to initialize config")
		return
	}
	if v.Initializer != nil {
		err = v.Initializer(v.config)
		if err != nil {
			return
		}
	}
	for _, applicability := range v.config.Applicability {
		if applicability == "" {
			err = fmt.Errorf("testSuite name cannot be an empty string")
			return
		}

		// NOTES
		// We need to be able to specify the policies — what catalogs are being run, and which applicability is applied
		// Applicability is currently a list, but it should be a string
		// Catalog should be a list
		// Policy is catalog + applicability
		// This will require changes all across the SDK and the plugin generator templates, but not to SCI yet.
		// We may move the Policy object type to SCI later if it seems useful.

		testSuite := layer4.EvaluationSuite{
			testSets:         v.Armory.EvaluationSuites[testSuiteName],
			executedTestSets: v.executedTestSets,
			config:           v.config,
		}
		testSuite.EvaluationSuite = testSuiteName // Inherited, can't be set above

		err = testSuite.Execute()

		v.EvaluationSuites = append(v.EvaluationSuites, testSuite)

		if testSuite.BadStateAlert {
			break
		}
	}
	v.config.Logger.Trace("Mobilization complete")

	if !v.config.Write {
		return
	}

	// loop through the testSuites and write the results
	for _, testSuite := range v.EvaluationSuites {
		err := testSuite.WriteControlEvaluations(v.ServiceName, v.config.Output)
		if err != nil {
			v.config.Logger.Error("Failed to write results for testSuite",
				"testSuite", testSuite.EvaluationSuite,
				"error", err,
			)
		}
	}
	return
}
