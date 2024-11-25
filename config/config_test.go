package config

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

// example config yaml objects

var testConfigs = []struct {
	testName       string
	runningService string
	requiredVars   []string
	missingVars    []string
	config         string
	logLevelSet    bool
	invasiveSet    bool
	writeDirSet    bool
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
		testName:       "Good - Log Level Set at Top Level",
		runningService: "my-service-1",
		requiredVars:   []string{},
		logLevelSet:    true,
		config: `
loglevel: debug
services:
  my-service-1:
    tactics:
      - tlp_green
      - tlp_clear
`}, {
		testName:       "Good - Log Level Set in Service",
		runningService: "my-service-1",
		requiredVars:   []string{},
		logLevelSet:    true,
		config: `
services:
  my-service-1:
    loglevel: debug
    tactics:
      - tlp_green
      - tlp_clear
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
      - tlp_clear
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
		config: `
services:
  my-service-1:
    tactics:
      - tlp_green
      - tlp_clear
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

			// create the config object
			viper.Set("service", tt.runningService)
			config := NewConfig(tt.requiredVars)
			if config.Error != nil {
				if len(tt.missingVars) > 0 {
					expectedError := fmt.Sprintf("missing required variables: %v", tt.missingVars)
					if config.Error.Error() != expectedError {
						t.Errorf("expected error to be '%s', got %v", expectedError, config.Error)
					}
				} else {
					t.Errorf("unexpected error: %v", config.Error)
				}
			} else if len(tt.missingVars) > 0 {
				t.Errorf("expected error for missing vars, got nil")
			}

			if config.ServiceName != tt.runningService {
				t.Errorf("expected service name to be '%s', got '%s'", tt.runningService, config.ServiceName)
			}
			if tt.logLevelSet && config.LogLevel == "Error" {
				t.Errorf("expected log level to be different from default, but got '%s'", config.LogLevel)
			} else if !tt.logLevelSet && config.LogLevel != "Error" {
				t.Errorf("expected log level to be set to default, but got '%s'", config.LogLevel)
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
