package pluginkit

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

// The vessel gets the armory in position to execute the testSets specified in the testSuites
type Vessel struct {
	ServiceName      string                     `json:"serviceName"`
	PluginName       string                     `json:"pluginName"`
	RequiredVars     []string                   `json:"requiredVars"`
	Armory           *Armory                    `json:"armory"`
	TestSuites       []TestSuite                `json:"testSuites"`
	Initializer      func(*config.Config) error `json:"initializer"`
	config           *config.Config
	logger           hclog.Logger
	executedTestSets *[]string
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
	if v.Armory.TestSuites == nil {
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
	for _, testSuiteName := range v.config.TestSuites {
		if testSuiteName == "" {
			err = fmt.Errorf("testSuite name cannot be an empty string")
			return
		}

		testSuite := TestSuite{
			TestSuiteName:    testSuiteName,
			testSets:         v.Armory.TestSuites[testSuiteName],
			executedTestSets: v.executedTestSets,
			config:           v.config,
		}

		err = testSuite.Execute()
		if testSuite.BadStateAlert {
			break
		}
		v.TestSuites = append(v.TestSuites, testSuite)
	}
	v.config.Logger.Trace("Mobilization complete")

	if !v.config.Write {
		return
	}

	// loop through the testSuites and write the results
	for _, testSuite := range v.TestSuites {
		err := testSuite.WriteTestSetResults(v.ServiceName, v.config.Output)
		if err != nil {
			v.config.Logger.Error("Failed to write results for testSuite",
				"testSuite", testSuite.TestSuiteName,
				"error", err,
			)
		}
	}
	return
}
