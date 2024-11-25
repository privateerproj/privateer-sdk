package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Config is a struct that contains the configuration for the raidengine
type Config struct {
	ServiceName    string // Must be unique in the config file
	LogLevel       string
	WriteDirectory string
	Tactics        []string
	Vars           map[string]interface{}
	Error          error
}

// NewConfig creates a new Config instance by reading configuration values using viper.
// It takes a slice of required variable names and checks if they are present in the configuration.
// If any required variables are missing, it returns an error listing the missing variables.
//
// Parameters:
//
//	[]string - A slice of strings representing the names of required variables.
//
// Returns:
//
//	*Config - A pointer to the created Config instance.
//	error - An error if any required variables are missing, otherwise nil.
func NewConfig(requiredVars []string) *Config {

	serviceName := viper.GetString("service") // the currently running service
	loglevel := viper.GetString("services." + serviceName + ".loglevel")
	if loglevel == "" {
		loglevel = viper.GetString("loglevel")
	}

	var errString string
	if len(viper.GetStringSlice("services."+serviceName+".tactics")) == 0 {
		errString = fmt.Sprintf("no tactics specified for service %s", serviceName)
	}

	var missingVars []string

	vars := viper.GetStringMap("services." + serviceName + ".vars")
	for _, v := range requiredVars {
		if _, ok := vars[v]; !ok {
			missingVars = append(missingVars, v)
		}
	}
	if len(missingVars) > 0 {
		errString = fmt.Sprintf("missing required variables: %v", missingVars)
	}

	var err error
	if errString != "" {
		err = errors.New(errString)
	}
	return &Config{
		ServiceName:    serviceName,
		LogLevel:       loglevel,
		WriteDirectory: viper.GetString("write-directory"),
		Tactics:        viper.GetStringSlice("services." + serviceName + ".tactics"),
		Vars:           vars,
		Error:          err,
	}
}
