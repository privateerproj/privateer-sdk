package pluginkit

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/gemaraproj/go-gemara"
	"github.com/gemaraproj/go-gemara/sarif"
	"github.com/goccy/go-yaml"
	"github.com/privateerproj/privateer-sdk/config"
)

// EvaluationOrchestrator gets the plugin in position to execute the specified evaluation suites.
type EvaluationOrchestrator struct {
	ServiceName       string             `yaml:"service-name"`
	PluginName        string             `yaml:"plugin-name"`
	PluginUri         string             `yaml:"plugin-uri"`
	PluginVersion     string             `yaml:"plugin-version"`
	Payload           any                `yaml:"payload,omitempty"`
	Evaluation_Suites []*EvaluationSuite `yaml:"evaluation-suites"` // EvaluationSuite is a map of evaluations to their catalog names

	possibleSuites    []*EvaluationSuite
	possibleControls  map[string][]*gemara.Control
	referenceCatalogs map[string]*gemara.Catalog
	requiredVars      []string
	config            *config.Config
	loader            DataLoader
}

// DataLoader is a function type for loading plugin data from configuration.
type DataLoader func(*config.Config) (any, error)

// AddLoader sets the data loader function for the orchestrator.
func (v *EvaluationOrchestrator) AddLoader(loader DataLoader) {
	v.loader = loader
}

// AddRequiredVars sets the required configuration variables for the orchestrator.
func (v *EvaluationOrchestrator) AddRequiredVars(vars []string) {
	v.requiredVars = vars
}

// AddReferenceCatalogs loads reference catalogs from the embedded file system.
func (v *EvaluationOrchestrator) AddReferenceCatalogs(dataDir string, files embed.FS) error {
	if v.referenceCatalogs == nil {
		v.referenceCatalogs = make(map[string]*gemara.Catalog)
	}
	if dataDir == "" {
		return errors.New("data directory name cannot be empty")
	}
	catalogs, err := getPluginCatalogs(dataDir, files)
	if err != nil {
		return err
	}
	for _, catalog := range catalogs {
		if catalog.Metadata.Id == "" {
			return errors.New("catalog id cannot be empty")
		}
		if _, exists := v.referenceCatalogs[catalog.Metadata.Id]; exists {
			return fmt.Errorf("duplicate catalog id found: %s", catalog.Metadata.Id)
		}
		v.referenceCatalogs[catalog.Metadata.Id] = catalog
		v.addPossibleControls(catalog)
	}
	return nil
}

func (v *EvaluationOrchestrator) addPossibleControls(catalog *gemara.Catalog) {
	if v.possibleControls == nil {
		v.possibleControls = make(map[string][]*gemara.Control)
	}
	for i := range catalog.Controls {
		control := &catalog.Controls[i]
		if _, exists := v.possibleControls[control.Id]; !exists {
			v.possibleControls[control.Id] = []*gemara.Control{control}
		} else {
			v.possibleControls[control.Id] = append(v.possibleControls[control.Id], control)
		}
	}
}

// AddEvaluationSuite adds an evaluation suite for the given catalog ID.
func (v *EvaluationOrchestrator) AddEvaluationSuite(catalogId string, loader DataLoader, steps map[string][]gemara.AssessmentStep) error {
	if catalogId == "" {
		return BAD_CATALOG(v.PluginName, "suite catalog id cannot be empty", "aos10")
	}
	if catalog, ok := v.referenceCatalogs[catalogId]; ok {
		if len(catalog.Controls) == 0 {
			return BAD_CATALOG(v.PluginName, "no controls provided", "aos20")
		}
		if catalog.Metadata.Id == "" {
			return BAD_CATALOG(v.PluginName, "no id found in catalog metadata", "aos30")
		}
		v.addEvaluationSuite(catalog, loader, steps)
		return nil
	}
	return BAD_CATALOG(v.PluginName, fmt.Sprintf("no reference catalog found with id '%s'", catalogId), "aos40")
}

func (v *EvaluationOrchestrator) addEvaluationSuite(catalog *gemara.Catalog, loader DataLoader, steps map[string][]gemara.AssessmentStep) {
	importedControlFamilies := getImportedControlFamilies(catalog, v.referenceCatalogs)
	catalog.Controls = append(catalog.Controls, importedControlFamilies...)

	suite := EvaluationSuite{
		CatalogId: catalog.Metadata.Id,
		catalog:   catalog,
		steps:     steps,
		config:    v.config,
	}

	if loader != nil {
		suite.loader = loader
	} else {
		suite.loader = v.loader
	}
	v.possibleSuites = append(v.possibleSuites, &suite)
}

// getImportedControlFamilies creates a new control family entry for each imported catalog
// and only includes controls from the imported catalog that are listed in the imports of the primary catalog.
func getImportedControlFamilies(catalog *gemara.Catalog, referenceCatalogs map[string]*gemara.Catalog) (importedControls []gemara.Control) {
	if len(catalog.ImportedControls) == 0 {
		return importedControls
	}
	for _, importEntry := range catalog.ImportedControls {
		// hacked this up, not sure about it yet
		if _, ok := referenceCatalogs[importEntry.ReferenceId]; ok {
			for _, mapping := range importEntry.Entries {
				if catalog, exists := referenceCatalogs[importEntry.ReferenceId]; exists {
					for _, control := range catalog.Controls {
						if control.Id == mapping.ReferenceId {
							importedControls = append(importedControls, control)
						}
					}
				}
			}
		}
	}
	return importedControls
}

// Mobilize initializes the orchestrator and executes all evaluation suites.
func (v *EvaluationOrchestrator) Mobilize() error {
	v.setupConfig()
	if v.config.Error != nil {
		return BAD_CONFIG(v.config.Error, "mob10")
	}

	if len(v.config.Policy.ControlCatalogs) == 0 {
		return BAD_CONFIG(v.config.Error, "mob20")
	}

	err := v.loadPayload()
	if err != nil {
		return BAD_LOADER(v.PluginName, err, "mob30")
	}

	v.ServiceName = v.config.ServiceName

	if v.PluginName == "" || v.ServiceName == "" {
		return EVALUATION_ORCHESTRATOR_NAMES_NOT_SET(v.ServiceName, v.PluginName, "mob40")
	}

	v.config.Logger.Trace("Mobilization beginning")
	if len(v.possibleSuites) == 0 {
		return NO_EVALUATION_SUITES("mob50")
	}

	for _, catalog := range v.config.Policy.ControlCatalogs {
		for _, suite := range v.possibleSuites {
			if suite.CatalogId == catalog {
				err := suite.Evaluate(v.ServiceName)
				if err != nil {
					v.config.Logger.Error(err.Error())
				}
				// Set plugin metadata in EvaluationLog for SARIF generation
				suite.EvaluationLog.Metadata = gemara.Metadata{
					Author: gemara.Actor{
						Name:    v.PluginName,
						Uri:     v.PluginUri,
						Version: v.PluginVersion,
					},
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

// WriteResults writes the evaluation results to files in the configured output format.
func (v *EvaluationOrchestrator) WriteResults() error {

	var err error
	var result []byte
	switch v.config.Output {
	case "json":
		result, err = json.Marshal(v)
		err = errMod(err, "wr10")
	case "yaml":
		result, err = yaml.Marshal(v)
		err = errMod(err, "wr20")
	case "sarif":
		for _, suite := range v.Evaluation_Suites {
			sarifBytes, sarifErr := sarif.FromEvaluationLog(suite.EvaluationLog, "", suite.catalog)
			if sarifErr != nil {
				err = errMod(sarifErr, "wr25")
				break
			}
			result = append(result, sarifBytes...)
		}
	default:
		err = fmt.Errorf("output type '%s' is not supported. Supported types are 'json', 'yaml', and 'sarif'", v.config.Output)
		err = errMod(err, "wr30")
	}
	if err != nil {
		return WRITE_FAILED(v.PluginName, err.Error(), "wr40")
	}
	err = v.writeResultsToFile(v.ServiceName, result, v.config.Output)
	if err != nil {
		return WRITE_FAILED(v.ServiceName, err.Error(), "wr60")
	}

	return nil
}

func (v *EvaluationOrchestrator) writeResultsToFile(serviceName string, result []byte, extension string) error {
	if !strings.Contains(extension, ".") {
		extension = fmt.Sprintf(".%s", extension)
	}
	dir := path.Join(v.config.WriteDirectory, serviceName)
	filename := fmt.Sprintf("%s%s", v.ServiceName, extension)
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

// loadPayload loads the payload data to be referenced in assessments.
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

		// Update all existing suites to point to the new config
		for _, suite := range v.possibleSuites {
			suite.config = v.config
		}
	}
}
