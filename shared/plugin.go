// Package shared provides shared functionality for Privateer plugins.
package shared

import (
	"errors"
	"net/rpc"

	hcplugin "github.com/hashicorp/go-plugin"
)

// Pluginer is the interface that we're exposing as a plugin. Start returns a
// privateer exit code (TestPass, TestFail, InternalError, BadUsage) and an
// optional error for diagnostic logging. Typed errors do not survive net/rpc,
// so classification must happen on the plugin side — see pluginkit.ExitCodeFor.
type Pluginer interface {
	Start() (int, error)
}

type StartResponse struct {
	ExitCode int
	Err      string
}

// PluginRPC is an implementation that talks over RPC.
type PluginRPC struct{ client *rpc.Client }

// Start is a wrapper for interface implementation of Start.
func (g *PluginRPC) Start() (int, error) {
	var resp StartResponse
	if err := g.client.Call("Plugin.Start", new(interface{}), &resp); err != nil {
		return InternalError, err
	}
	if resp.Err != "" {
		return resp.ExitCode, errors.New(resp.Err)
	}
	return resp.ExitCode, nil
}

// PluginRPCServer is the RPC server that PluginRPC talks to, conforming to
// the requirements of net/rpc.
type PluginRPCServer struct {
	// Impl is the real implementation.
	Impl Pluginer
}

// Start is a wrapper for interface implementation.
func (s *PluginRPCServer) Start(args interface{}, resp *StartResponse) error {
	code, err := s.Impl.Start()
	resp.ExitCode = code
	if err != nil {
		resp.Err = err.Error()
	}
	return nil
}

// Plugin is the implementation of plugin.Plugin so we can serve/consume this.
//
// This has two methods: Server must return an RPC server for this plugin
// type. We construct a PluginRPCServer for this.
//
// Client must return an implementation of our interface that communicates
// over an RPC client. We return PluginRPC for this.
//
// Ignore MuxBroker. That is used to create more multiplexed streams on our
// plugin connection and is a more advanced use case.
type Plugin struct {
	// Impl is the plugin implementation.
	Impl Pluginer
}

// Server implements RPC server.
func (p *Plugin) Server(*hcplugin.MuxBroker) (interface{}, error) {
	return &PluginRPCServer{Impl: p.Impl}, nil
}

// Client implements RPC client.
func (Plugin) Client(b *hcplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &PluginRPC{client: c}, nil
}
