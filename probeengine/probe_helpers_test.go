package probeengine

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/privateerproj/privateer-sdk/config"
)

func TestMain(m *testing.M) {

	log.Print("Initializing global test resources")

	os.MkdirAll(filepath.Join(testFolder()), 0755)

	defer func() {
		os.RemoveAll(testFolder())
		os.RemoveAll(config.GlobalConfig.TmpDir) // Delete test data after tests
	}()
	m.Run()
}

func testFolder() string {
	testFolder, _ := filepath.Abs("./testdata") // Need absolute path so that pkger.Open can work
	return testFolder
}

func TestGetOutputPath(t *testing.T) {
	var file *os.File
	config.GlobalConfig.WriteDirectory = t.TempDir()
	f := "test_file"
	desiredFile := filepath.Join(config.GlobalConfig.WriteDirectory, "cucumber", f+".json")

	defer func() {
		file.Close()
		// Swallow any panics and print a verbose error message
		if err := recover(); err != nil {
			t.Logf("Panicked when trying to create directory or file: '%s' || %v", desiredFile, file)
			t.Fail()
		}
	}()

	err := os.MkdirAll(filepath.Join(config.GlobalConfig.WriteDirectory, "cucumber"), 0755)
	if err != nil {
		t.Error(err)
	}

	file, _ = getOutputPath(f)
	if desiredFile != file.Name() {
		t.Logf("Desired filepath '%s' does not match '%s'", desiredFile, file.Name())
		t.Fail()
	}
}

func TestScenarioString(t *testing.T) {
	gs := &godog.Scenario{Name: "test scenario"}

	// Start scenario
	s := scenarioString(true, gs)
	sContainsString := strings.Contains(s, "Start")
	if !sContainsString {
		t.Logf("Test string does not contain 'Start'")
		t.Fail()
	}

	// End scenario
	s = scenarioString(false, gs)
	sContainsString = strings.Contains(s, "End")
	if !sContainsString {
		t.Logf("Test string does not contain 'End'")
		t.Fail()
	}
}

func TestGetFeaturePath(t *testing.T) {
	// Faking result for getTmpFeatureFileFunc() to avoid creating -tmp- folder and feature file.
	getTmpFeatureFileFunc = func(featurePath string) (string, error) {
		tmpFeaturePath := filepath.Join("tmp", featurePath)
		return tmpFeaturePath, nil
	}
	defer func() {
		getTmpFeatureFileFunc = getTmpFeatureFile //Restoring to original function after test
	}()

	type args struct {
		path []string
	}
	tests := []struct {
		testName       string
		testArgs       args
		expectedResult string
	}{
		{
			testName:       "GetFeaturePath_WithTwoSubfoldersAndFeatureName_ShouldReturnFeatureFilePath",
			testArgs:       args{path: []string{"internal", "container_registry_access"}},
			expectedResult: filepath.Join("tmp", "internal", "container_registry_access", "container_registry_access.feature"), // Using filepath.join() instead of literal string in order to run test in Windows (\\) and Linux (/)
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if got := GetFeaturePath(tt.testArgs.path...); got != tt.expectedResult {
				t.Errorf("GetFeaturePath() = %v, Expected: %v", got, tt.expectedResult)
			}
		})
	}
}

func Test_getTmpFeatureFile(t *testing.T) {
	config.GlobalConfig.TmpDir = t.TempDir()
	filename := "Test_getTmpFeatureFile.feature"
	os.Create(filepath.Join(testFolder(), filename))
	os.MkdirAll(filepath.Join(config.GlobalConfig.TmpDir, "probeengine", "testdata"), 0755)

	tests := []struct {
		testName       string
		featurePath    string
		expectedResult string
		expectedErr    bool
	}{
		{
			testName:       "ShouldCreateTmpFolderWithFeatureFile",
			featurePath:    filepath.Join("probeengine", "testdata", filename), // This cannot be an absolute path, since it will be joined with temp dir
			expectedResult: filepath.Join(config.GlobalConfig.TmpDir, "probeengine", "testdata", filename),
			expectedErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			got, err := getTmpFeatureFile(tt.featurePath)
			if err != nil {
				t.Error(err)
			}
			if got != tt.expectedResult {
				t.Errorf("getTmpFeatureFile() = %v, expected %v", got, tt.expectedResult)
			}
			// Check if file was saved to tmp location
			_, e := os.Stat(tt.expectedResult)
			if e != nil {
				t.Errorf("File not found in tmp location: %v - Error: %v", tt.expectedResult, e)
			}
		})
	}
}

func Test_unpackFileAndSave(t *testing.T) {
	filename := "Test_getTmpFeatureFile.feature"

	os.Create(filepath.Join(testFolder(), filename))

	type args struct {
		origFilePath string
		newFilePath  string
	}
	tests := []struct {
		testName    string
		testArgs    args
		expectedErr bool
	}{
		{
			testName: "ShouldCreateFileInNewLocation",
			testArgs: args{
				origFilePath: filepath.Join(testFolder(), filename),
				newFilePath:  filepath.Join(config.GlobalConfig.TmpDir, filename),
			},
			expectedErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if err := unpackFileAndSave(tt.testArgs.origFilePath, tt.testArgs.newFilePath); (err != nil) != tt.expectedErr {
				t.Errorf("unpackFileAndSave() error = %v, expected error: %v", err, tt.expectedErr)
			}
			// Check if file was saved to tmp location
			_, e := os.Stat(tt.testArgs.newFilePath)
			if e != nil {
				t.Errorf("File not found in tmp location: %v - Error: %v", tt.testArgs.newFilePath, e)
			}
		})
	}
}
