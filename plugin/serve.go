package plugin

import (
	"log"
	"os"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/privateerproj/privateer-sdk/logging"
)

const (
	// The constants below are the names of the plugins that can be dispensed
	// from the plugin server.

	// RaidPluginName ...
	RaidPluginName = "raid"
)

// handshakeConfigs are used to just do a basic handshake between
// a hcplugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad hcplugins or executing a hcplugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = GetHandshakeConfig()

// ServeOpts are the configurations to serve a plugin.
type ServeOpts struct {
	//Interface implementation
	Pack Raid

	// Logger is the logger that go-plugin will use.
	Logger hclog.Logger

	// Set NoLogOutputOverride to not override the log output with an hclog
	// adapter. This should only be used when running the plugin in
	// acceptance tests.
	NoLogOutputOverride bool
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve(opts *ServeOpts) {
	if !opts.NoLogOutputOverride {
		// In order to allow go-plugin to correctly pass log-levels through to
		// terraform, we need to use an hclog.Logger with JSON output. We can
		// inject this into the std `log` package here, so existing providers will
		// make use of it automatically.
		logger := logging.GetLogger("", nil, os.Stderr, true)
		log.SetOutput(logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true}))
	}

	// Plugin implementation
	// Guard Clause: Ensure plugin is not nil
	if opts.Pack == nil {
		log.Panic("Invalid (nil) plugin implementation provided")
	}
	raidPack := opts.Pack

	// hcpluginMap is the map of hcplugins we can dispense.
	var hcpluginMap = map[string]hcplugin.Plugin{
		RaidPluginName: &RaidPlugin{Impl: raidPack},
	}

	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         hcpluginMap,
		Logger:          opts.Logger,
	})
}

// GetHandshakeConfig provides handshake config details. It is used by core and service packs.
func GetHandshakeConfig() hcplugin.HandshakeConfig {

	return hcplugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "PVTR_MAGIC_COOKIE",
		MagicCookieValue: "privateer.raid",
	}
}
