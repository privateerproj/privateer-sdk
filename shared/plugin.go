package shared

import (
	"net/rpc"

	hcplugin "github.com/hashicorp/go-plugin"
)

// Plugin is the interface that we're exposing as a plugin.
type Pluginer interface {
	Start() error
}

// PluginRPC is an implementation that talks over RPC
type PluginRPC struct{ client *rpc.Client }

// Start is a wrapper for interface implementation of Start
func (g *PluginRPC) Start() error {
	var err error
	return g.client.Call("Plugin.Start", new(interface{}), &err)
}

// PluginRPCServer is the RPC server that PluginRPC talks to, conforming to
// the requirements of net/rpc
type PluginRPCServer struct {
	// This is the real implementation
	Impl Pluginer
}

// Start is a wrapper for interface implementation
func (s *PluginRPCServer) Start(args interface{}, resp *error) error {
	*resp = s.Impl.Start()
	return *resp
}

// Plugin is the implementation of plugin.Plugin so we can serve/consume this
//
// This has two methods: Server must return an RPC server for this plugin
// type. We construct a GreeterRPCServer for this.
//
// Client must return an implementation of our interface that communicates
// over an RPC client. We return GreeterRPC for this.
//
// Ignore MuxBroker. That is used to create more multiplexed streams on our
// plugin connection and is a more advanced use case.
type Plugin struct {
	// Impl Injection
	Impl Pluginer
}

// Server implements RPC server
func (p *Plugin) Server(*hcplugin.MuxBroker) (interface{}, error) {
	return &PluginRPCServer{Impl: p.Impl}, nil
}

// Client implements RPC client
func (Plugin) Client(b *hcplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &PluginRPC{client: c}, nil
}
