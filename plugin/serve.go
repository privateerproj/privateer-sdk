package plugin

import (
	"log"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
)

// RaidPluginName TODO: why did we put this here? what is this doing? Review and justify this.
const RaidPluginName = "raid"

// handshakeConfigs are used to just do a basic handshake between
// a hcplugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad hcplugins or executing a hcplugin
// directly. It is a UX feature, not a security feature.
var handshakeConfig = GetHandshakeConfig()

// ServeOpts are the configurations to serve a plugin.
type ServeOpts struct {
	//Interface implementation
	Plugin Raid

	// Logger is the logger that go-plugin will use.
	Logger hclog.Logger
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve(raidName string, opts *ServeOpts) {
	// Guard Clause: Ensure plugin is not nil
	if opts.Plugin == nil {
		log.Panic("Invalid (nil) plugin implementation provided")
	}

	// hcpluginMap is the map of hcplugins we can dispense.
	var hcpluginMap = map[string]hcplugin.Plugin{
		RaidPluginName: &RaidPlugin{Impl: opts.Plugin},
	}

	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         hcpluginMap,
		Logger:          opts.Logger,
	})
	log.Printf("Successfully completed raid: %s", raidName)
}

// GetHandshakeConfig provides handshake config details. It is used by core and service packs.
func GetHandshakeConfig() hcplugin.HandshakeConfig {
	return hcplugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "PVTR_MAGIC_COOKIE",
		MagicCookieValue: "privateer.raid",
	}
}
