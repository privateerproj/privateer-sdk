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
	logLevelExpected string
	invasiveSet      bool
	writeDirSet      bool
	expectedError    string
}{
	{
		testName:       "Good - One Service",
		runningService: "my-service-1",
		requiredVars:   []string{},
		config: `
services:
  my-service-1:
    tactics:
      - tlp_green
`}, {
		testName:       "Good - Two Services",
		runningService: "my-service-2",
		config: `
services:
  my-service-1:
    tactics:
      - tlp_green
  my-service-2:
    tactics:
      - tlp_green
      - tlp_clear
`}, {
		testName:         "Good - Log Level Set at Top Level",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		logLevelExpected: "debug",
		config: `
loglevel: debug
services:
  my-service-1:
    tactics:
      - tlp_green
`}, {
		testName:         "Good - Log Level Set in Service",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		logLevelExpected: "debug",
		config: `
services:
  my-service-1:
    loglevel: debug
    tactics:
      - tlp_green
`}, {
		testName:         "Good - Log Level Set in Service and Top Level",
		runningService:   "my-service-1",
		requiredVars:     []string{},
		logLevelExpected: "debug",
		config: `
loglevel: info
services:
  my-service-1:
    loglevel: debug
    tactics:
      - tlp_green
`}, {
		testName:       "Good - Invasive Set at Top Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		invasiveSet:    true,
		config: `
invasive: true
services:
  my-service-1:
    tactics:
      - tlp_green
`}, {
		testName:       "Good - Invasive Set at Service Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		invasiveSet:    true,
		config: `
services:
  my-service-1:
    invasive: true
    tactics:
      - tlp_green
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
    tactics:
      - tlp_green
`}, {
		testName:       "Good - Write Directory Set",
		runningService: "my-service-1",
		requiredVars:   []string{},
		writeDirSet:    true,
		config: `
write-directory: ./tmp
services:
  my-service-1:
    tactics:
      - tlp_green
      - tlp_clear
`}, {
		testName:       "Good - Required Var (Single)",
		runningService: "my-service-1",
		requiredVars:   []string{"key"},
		config: `
services:
  my-service-1:
    tactics:
      - tlp_green
    vars:
      key: value
`}, {
		testName:       "Good - Required Vars (Multiple)",
		runningService: "my-service-1",
		requiredVars:   []string{"key", "key2"},
		config: `
services:
  my-service-1:
    tactics:
      - tlp_green
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
    tactics:
      - tlp_green
      - tlp_clear
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
    tactics:
      - tlp_green
      - tlp_clear
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
    tactics:
      - tlp_green
      - tlp_clear
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
    tactics:
      - tlp_green
      - tlp_clear
    vars:
      key: value
`}, {
		testName:       "Bad - Missing Tactics",
		runningService: "my-service-1",
		requiredVars:   []string{},
		expectedError:  "no tactics requested for service in config: ",
		config: `
services:
  my-service-1:
    vars:
      key: value
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

			if tt.logLevelExpected != "" && config.LogLevel != tt.logLevelExpected {
				t.Errorf("expected log level to be set to '%s', but got '%s'", tt.logLevelExpected, config.LogLevel)
			}

			if tt.invasiveSet != config.Invasive {
				t.Errorf("expected invasive to be '%v', but got '%v'", tt.invasiveSet, config.Invasive)
			}

			if tt.writeDirSet && config.WriteDirectory == "" {
				t.Errorf("expected write directory to be set")
			} else if !tt.writeDirSet && config.WriteDirectory != defaultWritePath() {
				t.Errorf("expected write directory to be default, but got '%s'", config.WriteDirectory)
			}

			if len(config.Tactics) == 0 {
				t.Errorf("expected tactics to be set")
			}
		})
	}
}
