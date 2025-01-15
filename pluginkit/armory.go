package pluginkit

import (
	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

type Armory struct {
	PluginName    string               `json:"pluginName"`    // PluginName is the name of the plugin
	ServiceTarget string               `json:"serviceTarget"` // ServiceTarget is the name of the service the plugin is running on
	Config        *config.Config       `json:"config"`        // Config is the global configuration for the plugin
	Logger        hclog.Logger         `json:"logger"`        // Logger is the global logger for the plugin
	TestSuites    map[string][]TestSet `json:"testSuites"`    // TestSuites is a map of testSuite names to their testSets
	StartupFunc   func() error         `json:"startupFunc"`   // StartupFunc is a function to run before the testSets
	CleanupFunc   func() error         `json:"cleanupFunc"`   // CleanupFunc is a function to run after the testSets
}
