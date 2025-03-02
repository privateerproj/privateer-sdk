package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
)

// example config yaml objects

var testConfigs = []struct {
	testName         string
	runningService   string
	requiredVars     []string
	missingVars      []string
	config           string
	output           string
	invasiveSet      bool
	writeDirSet      bool
	writeSet         bool
	expectedLogLevel string
	expectedOutput   string
	expectedError    string
	expectedWrite    bool
}{
	{
		testName:       "Good - One Service",
		runningService: "my-service-1",
		requiredVars:   []string{},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Two Services",
		runningService: "my-service-2",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
  my-service-2:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:         "Good - Log Level Set at Top Level",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		expectedLogLevel: "debug",
		config: `
loglevel: debug
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:         "Good - Log Level Set in Service",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		expectedLogLevel: "debug",
		config: `
services:
  my-service-1:
    loglevel: debug
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:         "Good - Log Level Set in Service and Top Level",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		expectedLogLevel: "debug",
		config: `
loglevel: info
services:
  my-service-1:
    loglevel: debug
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Invasive Set at Top Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		invasiveSet:    true,
		config: `
invasive: true
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Invasive Set at Service Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		invasiveSet:    true,
		config: `
services:
  my-service-1:
    invasive: true
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Invasive Set at Service and Top Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		invasiveSet:    true,
		config: `
invasive: false
services:
  my-service-1:
    invasive: true
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Write Directory Set",
		runningService: "my-service-1",
		requiredVars:   []string{},
		writeDirSet:    true,
		config: `
write-directory: ./tmp
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - Required Var (Single)",
		runningService: "my-service-1",
		requiredVars:   []string{"key"},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
`}, {
		testName:       "Good - Required Vars (Multiple)",
		runningService: "my-service-1",
		requiredVars:   []string{"key", "key2"},
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
      key2: value2
`}, {
		testName:       "Bad - Missing Required Var (A)",
		runningService: "my-service-1",
		requiredVars:   []string{"missing"},
		missingVars:    []string{"missing"},
		expectedError:  "missing required variables: [missing]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
`}, {
		testName:       "Bad - Missing Required Var (B)",
		runningService: "my-service-1",
		requiredVars:   []string{"key", "missing"},
		missingVars:    []string{"missing"},
		expectedError:  "missing required variables: [missing]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
`}, {
		testName:       "Bad - Missing Required Vars (A)",
		runningService: "my-service-1",
		requiredVars:   []string{"missing1", "missing2"},
		missingVars:    []string{"missing1", "missing2"},
		expectedError:  "missing required variables: [missing1 missing2]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
`}, {
		testName:       "Bad - Missing Required Vars (B)",
		runningService: "my-service-1",
		requiredVars:   []string{"key", "missing1", "missing2"},
		missingVars:    []string{"missing1", "missing2"},
		expectedError:  "missing required variables: [missing1 missing2]",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
    vars:
      key: value
`}, {
		testName:       "Bad - Missing Policy",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedError:  "invalid policy for service my-service-1: ",
		config: `
services:
  my-service-1:
`}, {
		testName:       "Bad - Missing Applicability",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedError:  "invalid policy for service my-service-1: ",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
`}, {
		testName:       "Good - Default YAML output when missing",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedOutput: "yaml",
		config: `
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - designated output type JSON",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedOutput: "json",
		config: `
output: json
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - designated output type YAML",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedOutput: "yaml",
		config: `
output: yaml
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Bad - Bad output type",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedError:  "bad output type, allowed output types are json or yaml",
		config: `
output: bad
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - explicit write true",
		runningService: "my-service-1",
		requiredVars:   []string{},
		writeSet:       true,
		expectedWrite:  true,
		config: `
write: true
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - explicit write false",
		runningService: "my-service-1",
		requiredVars:   []string{},
		writeSet:       true,
		expectedWrite:  false,
		config: `
write: false
services:
  my-service-1:
    policy:
      catalogs:
        - FINOS-CCC
      applicability: tlp_green
`}, {
		testName:       "Good - write non boolean default to false false",
		runningService: "my-service-1",
		requiredVars:   []string{},
		writeSet:       true,
		expectedWrite:  false,
		config: `
write: blahblah
services:
  my-service-1:
    policy:
      catalogs: ["FINOS-CCC"]
      applicability: tlp_green
`},
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

			viper.Set("service", tt.runningService)
			config := NewConfig(tt.requiredVars)

			if config.Error == nil && tt.expectedError != "" {
				t.Errorf("expected error '%s', got nil", tt.expectedError)
				return
			} else if config.Error != nil && tt.expectedError == "" {
				t.Errorf("expected no error, got %v", config.Error)
				return
			} else if config.Error != nil && tt.expectedError != config.Error.Error() {
				t.Errorf("expected error '%s', got '%s'", tt.expectedError, config.Error.Error())
				return
			} else if config.Error != nil && tt.expectedError == config.Error.Error() {
				return
			}

			if config.ServiceName != tt.runningService {
				t.Errorf("expected service name to be '%s', got '%s'", tt.runningService, config.ServiceName)
			}

			if tt.invasiveSet != config.Invasive {
				t.Errorf("expected invasive to be '%v', but got '%v'", tt.invasiveSet, config.Invasive)
			}

			if tt.writeDirSet && config.WriteDirectory == "" {
				t.Errorf("expected write directory to be set")
			} else if !tt.writeDirSet && config.WriteDirectory != defaultWritePath() {
				t.Errorf("expected write directory to be default, but got '%s'", config.WriteDirectory)
			}

			if config.Policy.ControlCatalogs == nil || len(config.Policy.ControlCatalogs) == 0 {
				t.Errorf("expected policy to be set, but got %v", config.Policy)
			}

			if tt.expectedLogLevel != "" && config.LogLevel != tt.expectedLogLevel {
				t.Errorf("expected log level to be set to '%s', but got '%s'", tt.expectedLogLevel, config.LogLevel)
			}

			if tt.expectedOutput != "" && config.Output != tt.expectedOutput {
				t.Errorf("expected output to be '%s', but got '%s'", tt.expectedOutput, config.Output)
			}

			if tt.writeSet && tt.expectedWrite != config.Write {
				t.Errorf("expected write to be '%t', but got '%t'", tt.expectedWrite, config.Write)
			}
		})
	}
}
