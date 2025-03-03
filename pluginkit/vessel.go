package pluginkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/revanite-io/sci/pkg/layer4"
	"gopkg.in/yaml.v3"
)

// The vessel gets the armory in position to execute the ControlEvaluations specified in the testSuites
type Vessel struct {
	Service_Name      string
	Plugin_Name       string
	Payload           Payload
	Evaluation_Suites []*EvaluationSuite // EvaluationSuite is a map of evaluations to their catalog names

	possibleSuites []*EvaluationSuite
	requiredVars   []string
	config         *config.Config
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
		Plugin_Name:  pluginName,
		requiredVars: requiredVars,
	}
	v.SetPayload(payload)
	return v
}

// SetPayload allows the user to pass data to be referenced in assessments
func (v *Vessel) SetPayload(payload interface{}) {
	if payload == nil {
		payload = new(interface{})
	}
	v.Payload = Payload{
		Data: payload,
	}
}

func (v *Vessel) SetupConfig() {
	if v.config == nil {
		c := config.NewConfig(v.requiredVars)
		v.config = &c
	}
}

func (v *Vessel) AddEvaluationSuite(catalogId string, payload interface{}, evaluations []*layer4.ControlEvaluation) {
	suite := EvaluationSuite{
		Catalog_Id:          catalogId,
		Control_Evaluations: evaluations,
	}
	suite.config = v.config
	if payload != nil {
		suite.payload = payload
	} else {
		suite.payload = v.Payload.Data
	}
	v.possibleSuites = append(v.possibleSuites, &suite)
}

func (v *Vessel) Mobilize() error {
	v.SetupConfig()
	if v.config.Error != nil {
		return v.config.Error
	}

	v.config.Logger.Trace("Setting up vessel")

	if len(v.possibleSuites) == 0 {
		return NO_EVALUATION_SUITES()
	}

	v.Service_Name = v.config.ServiceName

	if v.Plugin_Name == "" || v.Service_Name == "" {
		return VESSEL_NAMES_NOT_SET(v.Service_Name, v.Plugin_Name)
	}

	v.config.Logger.Trace("Mobilization beginning")

	for _, catalog := range v.config.Policy.ControlCatalogs {
		for _, suite := range v.possibleSuites {
			if suite.Catalog_Id == catalog {
				suite.config = v.config
				evalName := v.Service_Name + "_" + catalog
				err := suite.Evaluate(evalName)
				if err != nil {
					v.config.Logger.Error(err.Error())
				}
				v.Evaluation_Suites = append(v.Evaluation_Suites, suite)
			}
		}
	}
	v.config.Logger.Trace("Mobilization complete")

	if !v.config.Write {
		return nil // Do not write results if the user has blocked it
	}

	// loop through the testSuites and write the results
	// for _, suite := range v.Evaluation_Suites {
	// 	suite.config = v.config
	// 	err := suite.WriteControlEvaluations(v.Service_Name, v.config.Output)
	// 	if err != nil {
	// 		v.config.Logger.Error(WRITE_FAILED(suite.Name, err.Error()).Error())
	// 	}
	// }

	// instead of writing the results one at a time, write everything to a single file
	return v.WriteResults()
}

func (v *Vessel) WriteResults() error {

	var err error
	var result []byte
	switch v.config.Output {
	case "json":
		result, err = json.Marshal(v)
	case "yaml":
		result, err = yaml.Marshal(v)
	default:
		err = fmt.Errorf("output type '%s' is not supported. Supported types are 'json' and 'yaml'", v.config.Output)
	}
	if err != nil {
		return err
	}

	err = v.writeResultsToFile(v.Service_Name, result, v.config.Output)
	if err != nil {
		return err
	}

	return nil
}

func (v *Vessel) writeResultsToFile(serviceName string, result []byte, extension string) error {
	if !strings.Contains(extension, ".") {
		extension = fmt.Sprintf(".%s", extension)
	}
	dir := path.Join(v.config.WriteDirectory, serviceName)
	filename := fmt.Sprintf("%s%s", v.Service_Name, extension)
	filepath := path.Join(dir, filename)

	v.config.Logger.Trace("Writing results", "filepath", filepath)

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			v.config.Logger.Error("Error creating directory", "directory", dir)
			return err
		}
		v.config.Logger.Warn("write directory for this plugin created for results, but should have been created when initializing logs", "directory", dir)
	}

	_, err := os.Create(filepath)
	if err != nil {
		v.config.Logger.Error("Error creating file", "filepath", filepath)
		return err
	}

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		v.config.Logger.Error("Error opening file", "filepath", filepath)
		return err
	}
	defer file.Close()

	_, err = file.Write(result)
	if err != nil {
		return err
	}

	return nil
}
