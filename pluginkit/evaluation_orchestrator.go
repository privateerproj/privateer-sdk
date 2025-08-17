package pluginkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/ossf/gemara/layer4"
	"github.com/privateerproj/privateer-sdk/config"
)

// The evaluation orchestrator gets the plugin in position to execute the specified evaluation suites
type EvaluationOrchestrator struct {
	Service_Name      string
	Plugin_Name       string
	Payload           interface{}
	Evaluation_Suites []*EvaluationSuite // EvaluationSuite is a map of evaluations to their catalog names

	possibleSuites []*EvaluationSuite
	requiredVars   []string
	config         *config.Config
	loader         DataLoader
}

type DataLoader func(*config.Config) (interface{}, error)

func NewEvaluationOrchestrator(pluginName string, loader DataLoader, requiredVars []string) *EvaluationOrchestrator {
	v := &EvaluationOrchestrator{
		Plugin_Name:  pluginName,
		requiredVars: requiredVars,
		loader:       loader,
	}
	return v
}

func (v *EvaluationOrchestrator) AddEvaluationSuite(catalogId string, loader DataLoader, evaluations []*layer4.ControlEvaluation) {
	suite := EvaluationSuite{
		Catalog_Id:          catalogId,
		Control_Evaluations: evaluations,
	}
	suite.config = v.config
	if loader != nil {
		suite.loader = loader
	} else {
		suite.loader = v.loader
	}
	v.possibleSuites = append(v.possibleSuites, &suite)
}

func (v *EvaluationOrchestrator) Mobilize() error {
	v.setupConfig()
	if v.config.Error != nil {
		return v.config.Error
	}
	err := v.loadPayload()
	if err != nil {
		return BAD_LOADER(v.Plugin_Name, err)
	}

	v.config.Logger.Trace("Setting up evaluation orchestrator")

	if len(v.possibleSuites) == 0 {
		return NO_EVALUATION_SUITES()
	}

	v.Service_Name = v.config.ServiceName

	if v.Plugin_Name == "" || v.Service_Name == "" {
		return EVALUATION_ORCHESTRATOR_NAMES_NOT_SET(v.Service_Name, v.Plugin_Name)
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
	return v.WriteResults()
}

func (v *EvaluationOrchestrator) WriteResults() error {

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

func (v *EvaluationOrchestrator) writeResultsToFile(serviceName string, result []byte, extension string) error {
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
	defer func() {
		_ = file.Close()
	}()

	_, err = file.Write(result)
	if err != nil {
		return err
	}

	return nil
}

// SetPayload allows the user to pass data to be referenced in assessments
func (v *EvaluationOrchestrator) loadPayload() (err error) {
	payload := new(interface{})
	if v.loader != nil {
		data, err := v.loader(v.config)
		if err != nil {
			return err
		}
		payload = &data
	}
	v.Payload = payload
	for _, suite := range v.possibleSuites {
		if suite.loader != nil {
			data, err := suite.loader(v.config)
			if err != nil {
				return err
			}
			suite.payload = data
		} else {
			suite.payload = v.Payload
		}
	}
	return nil
}

func (v *EvaluationOrchestrator) setupConfig() {
	if v.config == nil {
		c := config.NewConfig(v.requiredVars)
		v.config = &c
	}
}
