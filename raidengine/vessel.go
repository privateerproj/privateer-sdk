package raidengine

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

// The vessel gets the armory in position to execute the strikes specified in the tactics
type Vessel struct {
	ServiceName     string
	RaidName        string
	RequiredVars    []string
	Armory          *Armory
	Tactics         []Tactic
	Initializer     func(*config.Config) error
	config          *config.Config
	logger          hclog.Logger
	executedStrikes *[]string
}

// StockArmory sets up the armory for the vessel to use
func (v *Vessel) StockArmory() error {
	if v.Armory == nil {
		return errors.New("vessel's Armory field cannot be nil")
	}
	if v.logger == nil {
		if v.config == nil {
			config := config.NewConfig(v.RequiredVars)
			v.config = &config
		}
	}
	if v.config.Error != nil {
		return v.config.Error
	}

	v.Armory.Config = v.config
	v.Armory.Logger = v.config.Logger
	v.Armory.ServiceTarget = v.ServiceName

	v.logger = v.config.Logger
	v.ServiceName = v.config.ServiceName

	if v.RaidName == "" || v.ServiceName == "" {
		return fmt.Errorf("expected service and raid names to be set. ServiceName='%s' RaidName='%s'", v.ServiceName, v.RaidName)
	}
	if v.Armory == nil {
		return fmt.Errorf("no armory was stocked for the raid '%s'", v.RaidName)
	}
	if v.Armory.Tactics == nil {
		return fmt.Errorf("no tactics provided for the service")
	}

	return nil
}

// Mobilize executes the strikes specified in the tactics
func (v *Vessel) Mobilize() (err error) {
	err = v.StockArmory()
	if err != nil {
		return
	}
	if v.config == nil {
		err = fmt.Errorf("failed to initialize config")
		return
	}
	if v.Initializer != nil {
		err = v.Initializer(v.config)
		if err != nil {
			return
		}
	}
	for _, tacticName := range v.config.Tactics {
		if tacticName == "" {
			err = fmt.Errorf("tactic name cannot be an empty string")
			return
		}

		tactic := Tactic{
			TacticName:      tacticName,
			strikes:         v.Armory.Tactics[tacticName],
			executedStrikes: v.executedStrikes,
			config:          v.config,
		}

		err = tactic.Execute()
		if tactic.BadStateAlert {
			break
		}
		v.Tactics = append(v.Tactics, tactic)
	}
	v.config.Logger.Trace("Mobilization complete")

	// loop through the tactics and write the results
	for _, tactic := range v.Tactics {
		err := tactic.WriteStrikeResultsYAML(v.ServiceName)
		if err != nil {
			v.config.Logger.Error(fmt.Sprintf("Failed to write results for tactic '%s': %v", tactic.TacticName, err))
		}
	}
	return
}
