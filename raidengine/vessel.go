package raidengine

import (
	"fmt"
	"log"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/config"
)

// The vessel gets the armory in position to execute the strikes specified in the tactics
type Vessel struct {
	ServiceName     string
	RaidName        string
	Tactics         []Tactic
	config          *config.Config
	armory          *Armory
	logger          hclog.Logger
	executedStrikes *[]string
}

// StockArmory sets up the armory for the vessel to use
func (v *Vessel) StockArmory(armory *Armory) error {
	if armory == nil {
		return fmt.Errorf("armory cannot be nil")
	}
	if v.logger == nil {
		if v.config == nil {
			config := config.NewConfig(nil)
			v.config = &config
		}
	}
	if v.config.Error != nil {
		log.Printf("[ERROR] Failed to initialize the raid vessel: %v", v.config.Error.Error())
		return v.config.Error
	}

	armory.Config = v.config
	armory.Logger = v.config.Logger
	armory.ServiceTarget = v.ServiceName

	v.logger = v.config.Logger
	v.armory = armory
	v.ServiceName = v.config.ServiceName

	if v.RaidName == "" || v.ServiceName == "" {
		return fmt.Errorf("expected service and raid names to be set. ServiceName='%s' RaidName='%s'", v.ServiceName, v.RaidName)
	}
	if v.armory == nil {
		return fmt.Errorf("no armory was stocked for the raid '%s'", v.RaidName)
	}
	if v.armory.Tactics == nil {
		return fmt.Errorf("no tactics provided for the service")
	}

	return nil
}

// Mobilize executes the strikes specified in the tactics
func (v *Vessel) Mobilize() (err error) {
	if v.config == nil {
		err = fmt.Errorf("config cannot be nil")
		return
	}
	for _, tacticName := range v.config.Tactics {
		if tacticName == "" {
			err = fmt.Errorf("tactic name cannot be an empty string")
			return
		}

		tactic := Tactic{
			TacticName:      tacticName, // TODO: We should probably find a prettier way to name these
			strikes:         v.armory.Tactics[tacticName],
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
