package setter

import (
	"os"
	"reflect"
	"testing"
)

// TestSetStringVar ...
func TestSetStringVar(t *testing.T) {
	defaultValue := "default value"
	tests := []struct {
		testName            string
		varName             string
		envVarValue         string
		defaultValue        string
		expectedReturnValue string
	}{
		{
			testName:            "Test that set value is returned when provided",
			varName:             "ENV_VAR_1",
			envVarValue:         "env variable value #1",
			expectedReturnValue: "env variable value #1",
		},
		{
			testName:            "Test that default value is returned when no value is provided",
			varName:             "ENV_VAR_2",
			envVarValue:         "",
			expectedReturnValue: defaultValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			originalValue := os.Getenv(tt.varName) // Used to restore env to original state after test
			defer func() {
				os.Setenv(tt.varName, originalValue)
			}()

			os.Setenv(tt.varName, tt.envVarValue)

			vars := struct {
				Value string
			}{}
			value := setStringVar(vars.Value, tt.varName, defaultValue)

			if value != tt.expectedReturnValue {
				t.Errorf("setFromEnvOrDefaults(); Return Value = %v, Expected: %v", value, tt.expectedReturnValue)
				return
			}

		})
	}
}

// TestSetStringSliceVar ...
func TestSetStringSliceVar(t *testing.T) {
	defaultValue := []string{"one", "one", "two", "three", "five", "eight"}

	tests := []struct {
		testName            string
		varName             string
		envVarValue         string
		expectedReturnValue []string
	}{
		{
			testName:            "Test that set value is returned when provided",
			varName:             "ENV_VAR_1",
			envVarValue:         "one,two,three,four",
			expectedReturnValue: []string{"one", "two", "three", "four"},
		},
		{
			testName:            "Test that default value is returned when no value is provided",
			varName:             "ENV_VAR_2",
			envVarValue:         "",
			expectedReturnValue: defaultValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			originalValue := os.Getenv(tt.varName) // Used to restore env to original state after test
			defer func() {
				os.Setenv(tt.varName, originalValue)
			}()

			os.Setenv(tt.varName, tt.envVarValue)

			value := setStringSliceVar([]string{}, tt.varName, defaultValue)

			if !reflect.DeepEqual(value, tt.expectedReturnValue) {
				t.Errorf("setFromEnvOrDefaults(); Return Value = %v, Expected: %v", value, tt.expectedReturnValue)
				return
			}

		})
	}
}

func Test_setStringVar(t *testing.T) {
	defaultValuePVTRWRITEDIRECTORY := "privateer_output"
	envVarValuePVTRWRITEDIRECTORY := "ValueFromEnvVar_WriteDirectory"

	tests := []struct {
		testName            string
		varName             string
		envVarValue         string
		defaultValue        string
		expectedReturnValue string
	}{
		{
			testName:            "Test that set value is returned when provided",
			varName:             "ENV_VAR_1",
			envVarValue:         envVarValuePVTRWRITEDIRECTORY,
			defaultValue:        defaultValuePVTRWRITEDIRECTORY,
			expectedReturnValue: envVarValuePVTRWRITEDIRECTORY,
		},
		{
			testName:            "Test that default value is returned when no value is provided",
			varName:             "ENV_VAR_2",
			envVarValue:         "",
			defaultValue:        defaultValuePVTRWRITEDIRECTORY,
			expectedReturnValue: defaultValuePVTRWRITEDIRECTORY,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			originalValue := os.Getenv(tt.varName) // Used to restore env to original state after test
			defer func() {
				os.Setenv(tt.varName, originalValue)
			}()

			os.Setenv(tt.varName, tt.envVarValue)

			vars := struct {
				Value string
			}{}
			value := setStringVar(vars.Value, tt.varName, tt.defaultValue)

			if value != tt.expectedReturnValue {
				t.Errorf("setFromEnvOrDefaults(); Return Value = %v, Expected: %v", value, tt.expectedReturnValue)
				return
			}

		})
	}
}

func Test_setStringSliceVar(t *testing.T) {
	defaultValue := []string{"one", "one", "two", "three", "five", "eight"}

	tests := []struct {
		testName            string
		varName             string
		envVarValue         string
		expectedReturnValue []string
	}{
		{
			testName:            "Test that set value is returned when provided",
			varName:             "ENV_VAR_1",
			envVarValue:         "one,two,three,four",
			expectedReturnValue: []string{"one", "two", "three", "four"},
		},
		{
			testName:            "Test that default value is returned when no value is provided",
			varName:             "ENV_VAR_2",
			envVarValue:         "",
			expectedReturnValue: defaultValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			originalValue := os.Getenv(tt.varName) // Used to restore env to original state after test
			defer func() {
				os.Setenv(tt.varName, originalValue)
			}()

			os.Setenv(tt.varName, tt.envVarValue)

			value := setStringSliceVar([]string{}, tt.varName, defaultValue)

			if !reflect.DeepEqual(value, tt.expectedReturnValue) {
				t.Errorf("setFromEnvOrDefaults(); Return Value = %v, Expected: %v", value, tt.expectedReturnValue)
				return
			}

		})
	}
}
