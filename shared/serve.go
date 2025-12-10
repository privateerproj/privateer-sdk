package shared

import (
	"log"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
)

// PluginName is used by Privateer Core sally.go: rpcClient.Dispense(plugin.PluginName).
const PluginName = "plugin"

// handshakeConfig is used to just do a basic handshake between
// a hcplugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad hcplugins or executing a hcplugin
// directly. It is a UX feature, not a security feature.
var handshakeConfig = GetHandshakeConfig()

// ServeOpts are the configurations to serve a plugin.
type ServeOpts struct {
	// Plugin is the interface implementation.
	Plugin Pluginer

	// Logger is the logger that go-plugin will use.
	Logger hclog.Logger
}

// Serve serves a plugin. This function never returns and should be the final
// function called in the main function of the plugin.
func Serve(pluginName string, opts *ServeOpts) {
	// Guard Clause: Ensure plugin is not nil
	if opts.Plugin == nil {
		log.Panic("Invalid (nil) plugin implementation provided")
	}

	// hcpluginMap is the map of hcplugins we can dispense.
	var hcpluginMap = map[string]hcplugin.Plugin{
		PluginName: &Plugin{Impl: opts.Plugin},
	}

	hcplugin.Serve(&hcplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         hcpluginMap,
		Logger:          opts.Logger,
	})
	log.Printf("Successfully completed plugin: %s", pluginName)
}

// GetHandshakeConfig provides handshake config details.
// It is used by core and service packs.
func GetHandshakeConfig() hcplugin.HandshakeConfig {
	return hcplugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "PVTR_MAGIC_COOKIE_KEY",
		MagicCookieValue: "privateer.plugin",
	}
}
