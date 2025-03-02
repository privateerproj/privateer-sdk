package pluginkit

import (
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
)

// The vessel gets the armory in position to execute the testSets specified in the testSuites
type Vessel struct {
	ServiceName        string
	PluginName         string
	CatalogEvaluations map[string]EvaluationSuite // EvaluationSuite is a map of evaluations to their catalog names
	Payload            Payload

	requiredVars []string
	config       *config.Config
}

type Payload struct {
	Data   interface{}
	config *config.Config
}

func NewVessel(pluginName string, payload interface{}, requiredVars []string) *Vessel {
	if payload == nil {
		payload = new(interface{})
	}
	v := &Vessel{
		PluginName:   pluginName,
		requiredVars: requiredVars,
	}
	v.SetPayload(&payload)
	return v
}

// SetPayload allows the user to pass data to be referenced in assessments
func (v *Vessel) SetPayload(payload *interface{}) {
	if payload == nil {
		payload = new(interface{})
	}
	v.Payload = Payload{
		Data:   payload,
		config: v.config,
	}
}

func (v *Vessel) SetupConfig() {
	if v.config == nil {
		c := config.NewConfig(v.requiredVars)
		v.config = &c
	}
}

func (v *Vessel) AddEvaluationSuite(name string, payload *interface{}, evaluations []layer4.ControlEvaluation) {
	if v.CatalogEvaluations == nil {
		v.CatalogEvaluations = make(map[string]EvaluationSuite)
	}
	suite := EvaluationSuite{
		Name:                name,
		Control_Evaluations: evaluations,
	}
	if payload == nil {
		suite.payload = &v.Payload.Data
	}
	v.CatalogEvaluations[name] = suite
}

func (v *Vessel) Mobilize() error {
	v.SetupConfig()
	v.config.Logger.Trace("Setting up vessel")

	if v.CatalogEvaluations == nil || len(v.CatalogEvaluations) == 0 {
		return NO_EVALUATION_SUITES()
	}

	v.ServiceName = v.config.ServiceName

	if v.PluginName == "" || v.ServiceName == "" {
		return VESSEL_NAMES_NOT_SET(v.ServiceName, v.PluginName)
	}

	v.config.Logger.Trace("Mobilization beginning")

	for _, catalog := range v.config.Policy.ControlCatalogs {
		v.config.Logger.Trace("Running evaluations for catalog:", catalog)
		suite := v.CatalogEvaluations[catalog]
		suite.config = v.config
		evalName := v.ServiceName + "-" + catalog
		suite.Evaluate(evalName)
		if suite.Corrupted_State {
			v.config.Logger.Error(CORRUPTION_FOUND().Error())
		}
	}
	v.config.Logger.Trace("Mobilization complete")

	if !v.config.Write {
		return nil // Do not write results if the user has blocked it
	}

	// loop through the testSuites and write the results
	for _, suite := range v.CatalogEvaluations {
		suite.config = v.config
		err := suite.WriteControlEvaluations(v.ServiceName, v.config.Output)
		if err != nil {
			v.config.Logger.Error(WRITE_FAILED(suite.Name, err.Error()).Error())
		}
	}
	return nil
}
