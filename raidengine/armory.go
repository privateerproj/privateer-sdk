package raidengine

import (
	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

type Armory struct {
	RaidName      string
	ServiceTarget string
	Config        *config.Config
	Logger        hclog.Logger
	Tactics       map[string][]Strike
	StartupFunc   func() error
	CleanupFunc   func() error
}
