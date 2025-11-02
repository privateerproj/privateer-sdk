package config

import (
	"bytes"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

// example config yaml objects

var testConfigs = []struct {
	testName             string
	runningServiceName   string
	runningApplicability []string
	requiredVars         []string
	missingVars          []string
	config               string
	output               string
	invasiveSet          bool
	writeDirSet          bool
	writeSet             bool
	expectedLogLevel     string
	expectedOutput       string
	expectedError        string
	expectedWrite        bool
}{
	{
		testName:             "Good - One Service",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:           "Good - Two Services",
		runningServiceName: "my-service-2",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
  my-service-2:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Log Level Set at Top Level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedLogLevel:     "debug",
		config: `
loglevel: debug
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Log Level Set in Service",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedLogLevel:     "debug",
		config: `
services:
  my-service-1:
    loglevel: debug
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Log Level Set in Service and Top Level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedLogLevel:     "debug",
		config: `
loglevel: info
services:
  my-service-1:
    loglevel: debug
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Invasive Set at Top Level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		invasiveSet:          true,
		config: `
invasive: true
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Invasive Set at Service Level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		invasiveSet:          true,
		config: `
services:
  my-service-1:
    invasive: true
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Invasive Set at Service and Top Level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		invasiveSet:          true,
		config: `
invasive: false
services:
  my-service-1:
    invasive: true
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Write Directory Set",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		writeDirSet:          true,
		config: `
write-directory: ./tmp
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - Required Var (Single)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key"},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
`}, {
		testName:             "Good - Required Vars (Multiple)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key", "key2"},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
      key2: value2
`}, {
		testName:             "Bad - Missing Required Var (A)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"missing"},
		missingVars:          []string{"missing"},
		expectedError:        "missing required variables: [missing]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
`}, {
		testName:             "Bad - Missing Required Var (B)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key", "missing"},
		missingVars:          []string{"missing"},
		expectedError:        "missing required variables: [missing]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
`}, {
		testName:             "Bad - Missing Required Vars (A)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"missing1", "missing2"},
		missingVars:          []string{"missing1", "missing2"},
		expectedError:        "missing required variables: [missing1 missing2]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
`}, {
		testName:             "Bad - Missing Required Vars (B)",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key", "missing1", "missing2"},
		missingVars:          []string{"missing1", "missing2"},
		expectedError:        "missing required variables: [missing1 missing2]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
    vars:
      key: value
`}, {
		testName:             "Good - Policy at top level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		config: `
policy:
  catalogs:
    - FINOS-CCC
  applicability: ["tlp_green"]
services:
  my-service-1:
`},
	{
		testName:             "Bad - Missing Policy",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedError:        "invalid policy for service my-service-1. applicability=0 catalogs=0",
		config: `
services:
  my-service-1:
`}, {
		testName:             "Bad - Missing Applicability",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedError:        "invalid policy for service my-service-1. applicability=0 catalogs=1",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
`}, {
		testName:             "Good - Default YAML output when missing",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedOutput:       "yaml",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - designated output type JSON",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedOutput:       "json",
		config: `
output: json
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - designated output type YAML",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedOutput:       "yaml",
		config: `
output: yaml
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Bad - Bad output type",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		expectedError:        "bad output type, allowed output types are json or yaml",
		config: `
output: bad
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - explicit write true",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		writeSet:             true,
		expectedWrite:        true,
		config: `
write: true
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - explicit write false",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		writeSet:             true,
		expectedWrite:        false,
		config: `
write: false
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - write non boolean default to false false",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{},
		writeSet:             true,
		expectedWrite:        false,
		config: `
write: blahblah
services:
  my-service-1:
    policy:
      catalogs: ["FINOS-CCC"]
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - required vars set at top level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key"},
		config: `
vars:
  key: value
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]
`}, {
		testName:             "Good - required vars set at top and service level",
		runningServiceName:   "my-service-1",
		runningApplicability: []string{"tlp_green"},
		requiredVars:         []string{"key", "key2"},
		config: `
vars:
  key: value
services:
  my-service-1:
    vars:
      key2: value2
    policy:
      catalogs:
        - FINOS-CCC
      applicability: ["tlp_green"]`,
	},
}

func TestNewConfig(t *testing.T) {
	for _, tt := range testConfigs {
		t.Run(tt.testName, func(t *testing.T) {
			// setup viper with the test config
			viper.Reset()
			viper.SetConfigType("yaml")
			err := viper.ReadConfig(bytes.NewBufferString(tt.config))
			if err != nil {
				t.Fatalf("error reading config: %v", err)
			}

			viper.Set("service", tt.runningServiceName)
			c := NewConfig(tt.requiredVars)

			for _, foundApplicability := range tt.runningApplicability {
				var found bool
				for _, expectedApplicability := range c.Policy.Applicability {
					if foundApplicability == expectedApplicability {
						found = true
						break
					}
				}
				if !found && c.Error.Error() != tt.expectedError {
					t.Errorf("expected applicability to be '%v', got '%v'", tt.runningApplicability, c.Policy)
					break
				}
			}

			if c.Error == nil && tt.expectedError != "" {
				t.Errorf("expected error '%s', got nil", tt.expectedError)
				return
			} else if c.Error != nil && tt.expectedError == "" {
				t.Errorf("expected no error, got %v", c.Error)
				return
			} else if c.Error != nil && tt.expectedError != c.Error.Error() {
				t.Errorf("expected error '%s', got '%s'", tt.expectedError, c.Error.Error())
				return
			} else if c.Error != nil && tt.expectedError == c.Error.Error() {
				return
			}

			if c.ServiceName != tt.runningServiceName {
				t.Errorf("expected service name to be '%s', got '%s'", tt.runningServiceName, c.ServiceName)
			}

			if tt.invasiveSet != c.Invasive {
				t.Errorf("expected invasive to be '%v', but got '%v'", tt.invasiveSet, c.Invasive)
			}

			if tt.writeDirSet && c.WriteDirectory == "" {
				t.Errorf("expected write directory to be set")
			} else if !tt.writeDirSet && c.WriteDirectory != defaultWritePath() {
				t.Errorf("expected write directory to be default, but got '%s'", c.WriteDirectory)
			}

			if len(c.Policy.ControlCatalogs) == 0 {
				t.Errorf("expected policy to be set, but got %v", c.Policy)
			}

			if tt.expectedLogLevel != "" && c.LogLevel != tt.expectedLogLevel {
				t.Errorf("expected log level to be set to '%s', but got '%s'", tt.expectedLogLevel, c.LogLevel)
			}

			if tt.expectedOutput != "" && c.Output != tt.expectedOutput {
				t.Errorf("expected output to be '%s', but got '%s'", tt.expectedOutput, c.Output)
			}

			if tt.writeSet && tt.expectedWrite != c.Write {
				t.Errorf("expected write to be '%t', but got '%t'", tt.expectedWrite, c.Write)
			}
		})
	}
}
func TestDefaultWritePath(t *testing.T) {
	path := defaultWritePath()

	if path == "" {
		t.Error("expected defaultWritePath to return a non-empty string")
	}

	if !strings.Contains(path, ".privateer") {
		t.Error("expected path to contain '.privateer'")
	}

	if !strings.Contains(path, "logs") {
		t.Error("expected path to contain 'logs'")
	}
}

func TestPrintSanitizedVars(t *testing.T) {
	logger := hclog.NewNullLogger()
	vars := map[string]interface{}{
		"token":    "secret-token",
		"password": "my-password",
		"username": "testuser",
	}

	printSanitizedVars(logger, vars)
}

func TestSetupLogging(t *testing.T) {
	c := Config{
		WriteDirectory: "/tmp/test",
		Write:          false,
		LogLevel:       "Error",
	}

	c.SetupLogging("test-service", false)

	if c.Logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestSetupLoggingFilesAndDirectories(t *testing.T) {
	tmpDir := path.Join(os.TempDir(), "privateer-test")
	defer func() {
	     err := os.RemoveAll(tmpDir)
	     if err != nil {
	         t.Error("Failed to clean up tmpDir")
	     }
	     return
	 }

	logFilePath := path.Join(tmpDir, "test", "service.log")

	config := Config{
		WriteDirectory: tmpDir,
		Write:          true,
		LogLevel:       "Error",
	}

	writer := config.setupLoggingFilesAndDirectories(logFilePath)

	if writer == nil {
		t.Error("expected writer to be set")
	}
}
