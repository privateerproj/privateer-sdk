package pluginkit

import (
	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

type Armory struct {
	PluginName    string
	ServiceTarget string
	Config        *config.Config
	Logger        hclog.Logger
	TestSuites    map[string][]TestSet
	StartupFunc   func() error
	CleanupFunc   func() error
}
