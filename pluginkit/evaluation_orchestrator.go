package pluginkit

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/gemaraproj/go-gemara/gemaraconv"
	"github.com/goccy/go-yaml"
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
)

// EvaluationOrchestrator gets the plugin in position to execute the specified evaluation suites.
type EvaluationOrchestrator struct {
	ServiceName   string `json:"service-name" yaml:"service-name"`
	PluginName    string `json:"plugin-name" yaml:"plugin-name"`
	PluginUri     string `json:"plugin-uri" yaml:"plugin-uri"`
	PluginVersion string `json:"plugin-version" yaml:"plugin-version"`
	// Publisher is the grc.store author/org id that owns this plugin (and, by
	// convention, the catalogs it evaluates). With PluginName it forms the publish
	// coordinate <Publisher>/<PluginName>, and it is the namespace of every
	// evaluated catalog in the publish manifest. Inert at run time; required only
	// to publish (a plugin runs without it, but cannot be published without it).
	Publisher string `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	// License is the plugin's publication license as an SPDX expression
	// (e.g. "Apache-2.0", or "Apache-2.0 OR MIT"; use a LicenseRef-… token for a
	// custom license). grc.store requires it on every publication and it is
	// recorded in the signed plugin config. Like Publisher it is inert at run time
	// and required only to publish; pvtr validates and canonicalizes it (via
	// grc-store-protocol/spdx) at publish time, so the plugin only declares the
	// raw string here.
	License string `json:"license,omitempty" yaml:"license,omitempty"`
	// CatalogNamespaces optionally maps a reference-catalog id to the grc.store
	// namespace that owns it, for plugins that evaluate catalogs published by
	// someone else (e.g. a community plugin evaluating ossf/osps-baseline).
	// Catalogs not listed here are assumed to live under Publisher.
	CatalogNamespaces map[string]string  `json:"catalog-namespaces,omitempty" yaml:"catalog-namespaces,omitempty"`
	Payload           any                `json:"payload,omitempty" yaml:"payload,omitempty"`
	Evaluation_Suites []*EvaluationSuite `json:"evaluation-suites" yaml:"evaluation-suites"` // EvaluationSuite is a map of evaluations to their catalog names

	possibleSuites    []*EvaluationSuite
	possibleControls  map[string][]*gemara.Control
	referenceCatalogs map[string]*gemara.ControlCatalog
	requiredVars      []string
	config            *config.Config
	loader            DataLoader
	targetBuilder     TargetBuilder
}

// DataLoader is a function type for loading plugin data from configuration.
type DataLoader func(*config.Config) (any, error)

// TargetBuilder describes the resource a run evaluated, for the target field
// of the emitted gemara EvaluationLog. It receives the resolved config so it
// can derive real-world identity from service vars (e.g. "owner/repo" for a
// GitHub repository plugin). When no builder is registered the target falls
// back to the service name from the run configuration.
type TargetBuilder func(*config.Config) gemara.Resource

// AddLoader sets the data loader function for the orchestrator.
func (v *EvaluationOrchestrator) AddLoader(loader DataLoader) {
	v.loader = loader
}

// AddTargetBuilder sets the function that identifies the evaluated resource
// in each emitted EvaluationLog. Fields left empty by the builder are
// backfilled from the service name so the log always identifies its target.
func (v *EvaluationOrchestrator) AddTargetBuilder(builder TargetBuilder) {
	v.targetBuilder = builder
}

// AddRequiredVars sets the required configuration variables for the orchestrator.
func (v *EvaluationOrchestrator) AddRequiredVars(vars []string) {
	v.requiredVars = vars
}

// AddReferenceCatalogs loads reference catalogs from the embedded file system.
func (v *EvaluationOrchestrator) AddReferenceCatalogs(dataDir string, files embed.FS) error {
	if v.referenceCatalogs == nil {
		v.referenceCatalogs = make(map[string]*gemara.ControlCatalog)
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

func (v *EvaluationOrchestrator) addPossibleControls(catalog *gemara.ControlCatalog) {
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

// AddEvaluationSuiteForAllCatalogs registers the provided steps for every
// reference catalog that has been loaded via AddReferenceCatalogs.
// This allows plugin developers to define their step implementations once and
// have them automatically applied to all catalog versions.
func (v *EvaluationOrchestrator) AddEvaluationSuiteForAllCatalogs(loader DataLoader, steps map[string][]gemara.AssessmentStep) error {
	if len(v.referenceCatalogs) == 0 {
		return BAD_CATALOG(v.PluginName, "no reference catalogs loaded", "aac10")
	}
	for catalogId := range v.referenceCatalogs {
		if err := v.AddEvaluationSuite(catalogId, loader, steps); err != nil {
			return err
		}
	}
	return nil
}

func (v *EvaluationOrchestrator) addEvaluationSuite(catalog *gemara.ControlCatalog, loader DataLoader, steps map[string][]gemara.AssessmentStep) {
	for _, existing := range v.possibleSuites {
		if existing.CatalogId == catalog.Metadata.Id {
			return
		}
	}

	importedControls := getImportedControls(catalog, v.referenceCatalogs)
	suiteCatalog := catalog
	if len(importedControls) > 0 {
		// Copy-on-import: the suite evaluates its own + imported controls, but the
		// shared referenceCatalogs entry must stay pristine — PublishManifest reads
		// it, and the published requirement_ids must list only the catalog's OWN
		// control ids, deterministically, regardless of suite registration order.
		combined := *catalog
		combined.Controls = append(append([]gemara.Control{}, catalog.Controls...), importedControls...)
		suiteCatalog = &combined
	}

	suite := EvaluationSuite{
		CatalogId: catalog.Metadata.Id,
		catalog:   suiteCatalog,
		steps:     steps,
		config:    v.config,
	}

	// Leave suite.loader nil when no override is given so loadPayload
	// reuses the orchestrator payload instead of re-running v.loader per suite.
	if loader != nil {
		suite.loader = loader
	}
	v.possibleSuites = append(v.possibleSuites, &suite)
}

// getImportedControls returns controls from imported catalogs that are listed in the primary catalog's imports.
func getImportedControls(catalog *gemara.ControlCatalog, referenceCatalogs map[string]*gemara.ControlCatalog) []gemara.Control {
	if len(catalog.Imports) == 0 {
		return nil
	}
	var result []gemara.Control
	for _, importEntry := range catalog.Imports {
		refCatalog, ok := referenceCatalogs[importEntry.ReferenceId]
		if !ok {
			continue
		}
		for _, mapping := range importEntry.Entries {
			for i := range refCatalog.Controls {
				c := refCatalog.Controls[i]
				if c.Id == mapping.ReferenceId {
					result = append(result, c)
					break
				}
			}
		}
	}
	return result
}

// Mobilize initializes the orchestrator and executes all evaluation suites.
func (v *EvaluationOrchestrator) Mobilize() error {
	v.Evaluation_Suites = nil
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

	availableCatalogIDs := make([]string, 0, len(v.possibleSuites))
	for _, suite := range v.possibleSuites {
		availableCatalogIDs = append(availableCatalogIDs, suite.CatalogId)
	}

	for _, catalog := range v.config.Policy.ControlCatalogs {
		matched := false
		for _, suite := range v.possibleSuites {
			if suite.CatalogId == catalog {
				matched = true
				err := suite.Evaluate(v.ServiceName)
				if err != nil {
					v.config.Logger.Error(err.Error())
				}
				v.stampEvaluationLog(suite)
				v.Evaluation_Suites = append(v.Evaluation_Suites, suite)
				break
			}
		}
		if !matched {
			v.config.Logger.Warn("requested catalog did not match any available suite", "requested", catalog, "available", availableCatalogIDs)
		}
	}

	if len(v.Evaluation_Suites) == 0 {
		return NO_MATCHING_CATALOGS(v.config.Policy.ControlCatalogs, availableCatalogIDs, "mob60")
	}

	v.config.Logger.Trace("Mobilization complete")

	if !v.config.Write {
		return nil // Do not write results if the user has blocked it
	}
	return v.WriteResults()
}

// stampEvaluationLog populates identity, provenance, and outcome on a suite's
// EvaluationLog after evaluation, so the log is self-describing and
// schema-valid when emitted standalone (output: gemara) rather than inside
// the Privateer orchestrator envelope. The Author metadata also feeds SARIF
// generation.
func (v *EvaluationOrchestrator) stampEvaluationLog(suite *EvaluationSuite) {
	description := fmt.Sprintf("Privateer evaluation of service '%s' against control catalog '%s'", v.ServiceName, suite.CatalogId)
	if suite.CorruptedState {
		// The gemara EvaluationLog schema has no corrupted-state field, so this
		// stable marker in the description is how a standalone log reports that
		// an invasive change failed to revert. Keep the marker text machine-matchable.
		description += "; corrupted-state: true (an invasive change failed to revert and the target may be in a bad state)"
	}

	// The author id is the grc.store publish coordinate when a Publisher is
	// declared, otherwise the plugin name.
	authorId := v.PluginName
	if v.Publisher != "" {
		authorId = fmt.Sprintf("%s/%s", v.Publisher, v.PluginName)
	}

	suite.EvaluationLog.Metadata = gemara.Metadata{
		Id:            fmt.Sprintf("%s_%s", v.ServiceName, suite.CatalogId),
		Type:          gemara.EvaluationLogArtifact,
		GemaraVersion: gemara.SchemaVersion,
		Date:          gemara.Datetime(time.Now().UTC().Format(time.RFC3339)),
		Description:   description,
		Author: gemara.Actor{
			Id:      authorId,
			Name:    v.PluginName,
			Type:    gemara.Software,
			Uri:     v.PluginUri,
			Version: v.PluginVersion,
		},
	}

	// The suite aggregates per-evaluation results; mirror that onto the log so
	// a standalone log carries its overall outcome.
	suite.EvaluationLog.Result = suite.Result

	target := gemara.Resource{
		Id:   v.ServiceName,
		Name: v.ServiceName,
		Type: gemara.Software,
	}
	if v.targetBuilder != nil {
		built := v.targetBuilder(v.config)
		if built.Id == "" {
			built.Id = target.Id
		}
		if built.Name == "" {
			built.Name = target.Name
		}
		if built.Type == gemara.InvalidEntityType {
			built.Type = target.Type
		}
		target = built
	}
	suite.EvaluationLog.Target = target
}

// WriteResults writes the evaluation results to files in the configured output format.
func (v *EvaluationOrchestrator) WriteResults() error {

	// The orchestrator's Payload is typically very large and is only useful for tracing.
	// Omit it from results unless the user explicitly opts in via --include-payload.
	// The omitempty struct tag on Payload then drops the field entirely from yaml/json.
	// EvaluationSuite.payload is unexported and not serialized, so no per-suite cleanup is needed.
	// Snapshot-and-restore keeps WriteResults idempotent: callers that invoke it more than once,
	// or that inspect the orchestrator afterwards, still see the original payload.
	if !v.config.IncludePayload {
		saved := v.Payload
		v.Payload = nil
		defer func() { v.Payload = saved }()
	}

	var err error
	var result []byte
	switch v.config.Output {
	case "json":
		result, err = json.Marshal(v)
		err = errMod(err, "wr10")
	case "yaml":
		result, err = yaml.Marshal(v)
		err = errMod(err, "wr20")
	case "gemara":
		// Emit gemara-native EvaluationLog objects so results are directly
		// consumable by the wider gemara ecosystem. One log per suite is
		// produced; unlike the default formats this does not wrap them in
		// Privateer's orchestrator envelope.
		result, err = v.marshalGemara()
		err = errMod(err, "wr25")
	case "sarif":
		for _, suite := range v.Evaluation_Suites {
			evalConverter := gemaraconv.EvaluationLog(suite.EvaluationLog)
			sarifBytes, sarifErr := evalConverter.ToSARIF(gemaraconv.WithCatalog(suite.catalog))
			if sarifErr != nil {
				break
			}
			result = append(result, sarifBytes...)
		}
	default:
		err = fmt.Errorf("output type '%s' is not supported. Supported types are 'json', 'yaml', 'sarif', and 'gemara'", v.config.Output)
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

// marshalGemara serializes the run as gemara-native EvaluationLog objects.
// Each evaluation suite contributes one EvaluationLog. Output is always a list,
// even for a single suite, so downstream gemara consumers can parse uniformly.
func (v *EvaluationOrchestrator) marshalGemara() ([]byte, error) {
	logs := make([]gemara.EvaluationLog, 0, len(v.Evaluation_Suites))
	for _, suite := range v.Evaluation_Suites {
		logs = append(logs, suite.EvaluationLog)
	}
	return yaml.Marshal(logs)
}

func (v *EvaluationOrchestrator) writeResultsToFile(serviceName string, result []byte, extension string) error {
	// gemara output is YAML-encoded; write it with a .yaml extension
	// rather than a literal ".gemara" so tooling recognizes the format.
	if extension == "gemara" {
		extension = "yaml"
	}
	if !strings.Contains(extension, ".") {
		extension = fmt.Sprintf(".%s", extension)
	}
	dir := path.Join(v.config.WriteDirectory, serviceName)
	filename := fmt.Sprintf("%s%s", v.ServiceName, extension)
	filepath := path.Join(dir, filename)

	v.config.Logger.Trace("Writing results", "filepath", filepath)

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, utils.DirPermissions)
		if err != nil {
			v.config.Logger.Error("Error creating directory", "directory", dir)
			return err
		}
		v.config.Logger.Warn("write directory for this plugin created for results, but should have been created when initializing logs", "directory", dir)
	}

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
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
	if v.loader != nil {
		data, err := v.loader(v.config)
		if err != nil {
			return err
		}
		v.Payload = data
	}
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
