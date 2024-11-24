package config

import (
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
}

// NewConfig creates a new Config struct with the provided data
func NewConfig(requiredVars []string) (*Config, error) {

	serviceName := viper.GetString("service") // the currently running service
	loglevel := viper.GetString("services." + serviceName + ".loglevel")
	// if loglevel == "" {
	// 	loglevel = viper.GetString("loglevel")
	// }

	var missingVars []string

	vars := viper.GetStringMap("services." + serviceName + ".vars")
	for _, v := range requiredVars {
		if _, ok := vars[v]; !ok {
			missingVars = append(missingVars, v)
		}
	}
	var err error
	if len(missingVars) > 0 {
		err = fmt.Errorf("missing required variables: %v", missingVars)
	}

	return &Config{
		ServiceName:    serviceName,
		LogLevel:       loglevel,
		WriteDirectory: viper.GetString("WriteDirectory"),
		Tactics:        viper.GetStringSlice("services." + serviceName + ".tactics"),
		Vars:           vars,
	}, err
}
