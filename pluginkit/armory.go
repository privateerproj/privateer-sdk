package pluginkit

import (
	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

type Armory struct {
	PluginName    string               // PluginName is the name of the plugin
	ServiceTarget string               // ServiceTarget is the name of the service the plugin is running on
	Config        *config.Config       // Config is the global configuration for the plugin
	Logger        hclog.Logger         // Logger is the global logger for the plugin
	TestSuites    map[string][]TestSet // TestSuites is a map of testSuite names to their testSets
	StartupFunc   func() error         // StartupFunc is a function to run before the testSets
	CleanupFunc   func() error         // CleanupFunc is a function to run after the testSets
}
