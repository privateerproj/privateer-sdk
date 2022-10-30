package config

import (
	"time"

	ac "github.com/privateerproj/privateer-sdk/providers/azure/config"
)

// CloudProviders config options
type CloudProviders struct {
	Azure ac.Azure `yaml:"Azure"`
}

// GlobalOpts provides configurable options that will be used throughout the SDK
type GlobalOpts struct {
	StartTime          time.Time
	VarsFile           string
	InstallDir         string         `yaml:"InstallDir"`
	TmpDir             string         `yaml:"TmpDir"`
	GodogResultsFormat string         `yaml:"GodogResultsFormat"`
	CloudProviders     CloudProviders `yaml:"CloudProviders"`
	WriteDirectory     string         `yaml:"WriteDirectory"`
	LogLevel           string         `yaml:"LogLevel"`
	TagExclusions      []string       `yaml:"TagExclusions"`
	TagInclusions      []string       `yaml:"TagInclusions"`
	WriteConfig        string         `yaml:"WriteConfig"`
}
