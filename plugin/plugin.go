package plugin

import (
	"net/rpc"

	hcplugin "github.com/hashicorp/go-plugin"
)

// Raid is the interface that we're exposing as a plugin.
type Raid interface {
	Start() error
}

// RaidRPC is an implementation that talks over RPC
type RaidRPC struct{ client *rpc.Client }

// Start returns a message
func (g *RaidRPC) Start() error {
	var err error
	return g.client.Call("Plugin.Start", new(interface{}), &err)
}

// RaidRPCServer is the RPC server that RaidRPC talks to, conforming to
// the requirements of net/rpc
type RaidRPCServer struct {
	// This is the real implementation
	Impl Raid
}

// Start is a wrapper for interface implementation
func (s *RaidRPCServer) Start(args interface{}, resp *error) error {
	*resp = s.Impl.Start()
	return *resp
}

// RaidPlugin is the implementation of plugin.Plugin so we can serve/consume this
//
// This has two methods: Server must return an RPC server for this plugin
// type. We construct a GreeterRPCServer for this.
//
// Client must return an implementation of our interface that communicates
// over an RPC client. We return GreeterRPC for this.
//
// Ignore MuxBroker. That is used to create more multiplexed streams on our
// plugin connection and is a more advanced use case.
type RaidPlugin struct {
	// Impl Injection
	Impl Raid
}

// Server implements RPC server
func (p *RaidPlugin) Server(*hcplugin.MuxBroker) (interface{}, error) {
	return &RaidRPCServer{Impl: p.Impl}, nil
}

// Client implements RPC client
func (RaidPlugin) Client(b *hcplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &RaidRPC{client: c}, nil
}
