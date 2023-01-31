package probeengine

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
)

// Probe is an interface used by probes that are to be exported from any service pack
type Probe interface {
	ProbeInitialize(*godog.TestSuiteContext)
	ScenarioInitialize(*godog.ScenarioContext)
	Name() string
	Path() string
}

// see TestGetOutputPath
var getTmpFeatureFileFunc = getTmpFeatureFile // See TestGetFeaturePath

// getOutputPath gets the output path for the test based on the output directory
// plus the test name supplied
func getOutputPath(name string) (*os.File, error) {
	filename := name + ".json"
	return os.Create(filepath.Join(
		config.GlobalConfig.WriteDirectory, "cucumber", filename))
}

// GetFilePath parses a list of strings into a standardized file path. The filename should be in the final element of path
func GetFilePath(path ...string) (filePath string) {
	for _, entry := range path {
		filePath = filepath.Join(filePath, entry)
	}

	// Unpacking/copying feature file to tmp location
	tmpFilePath, err := getTmpFeatureFileFunc(filePath)
	if err != nil {
		log.Printf("Error unpacking feature file '%v' - Error: %v", filePath, err)
		return ""
	}
	return tmpFilePath
}

// GetFeaturePath parses a list of strings into a standardized file path for the BDD ".feature" files
// TODO: refactor this to use GetFilePath
func GetFeaturePath(path ...string) string {
	featureName := path[len(path)-1] + ".feature"
	path = append(path, featureName)
	return GetFilePath(path...)
}

func getTmpFeatureFile(featurePath string) (string, error) {
	// indev 0.0.1 - removed pkger logic here
	return "", nil
}

func unpackFileAndSave(origFilePath string, newFilePath string) error {

	// TODO: This function could be extracted to a separate object i.e: Bundler interface?

	fileBytes, readFileErr := utils.ReadStaticFile(origFilePath) // Read bytes using pkger memory bundle
	if readFileErr != nil {
		return fmt.Errorf("Error reading file content: '%v' - Error: %v", origFilePath, readFileErr)
	}

	createFilePathErr := os.MkdirAll(filepath.Dir(newFilePath), 0755) // Create directory and sub directories for file
	if createFilePathErr != nil {
		return fmt.Errorf("Error creating path for file: '%v' - Error: %v", newFilePath, createFilePathErr)
	}

	writeFileErr := ioutil.WriteFile(newFilePath, fileBytes, 0755) // Save file to new location
	if writeFileErr != nil {
		return fmt.Errorf("Error saving file: '%v' - Error: %v", newFilePath, writeFileErr)
	}

	return nil // File created
}

// LogScenarioStart logs the name and tags associated with the supplied scenario.
func LogScenarioStart(s *godog.Scenario) {
	log.Print(scenarioString(true, s))
}

// LogScenarioEnd logs the name and tags associated with the supplied scenario.
func LogScenarioEnd(s *godog.Scenario) {
	log.Print(scenarioString(false, s))
}

func scenarioString(st bool, s *godog.Scenario) string {
	var b strings.Builder
	if st {
		b.WriteString("[INFO] >>> Scenario Start: ")
	} else {
		b.WriteString("[INFO] <<< Scenario End: ")
	}

	b.WriteString(s.Name)
	b.WriteString(". (Tags: ")

	for _, t := range s.Tags {
		b.WriteString(t.GetName())
		b.WriteString(" ")
	}
	b.WriteString(").")
	return b.String()
}
