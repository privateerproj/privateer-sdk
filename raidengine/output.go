package raidengine

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v3"
)

// GetLogger returns an hc logger with the provided name.
// It will creates or update the logger to use the provided level and format.
// If the logger already exists, it will return the existing logger.
// For level options, reference:
// https://github.com/hashicorp/go-hclog/blob/master/logger.go#L19
func GetLogger(name string, jsonFormat bool) hclog.Logger {
	// Initialize file writer for MultiWriter
	var filepath string
	if name == "overview" {
		// if this is not a raid, do not nest within a directory
		filepath = path.Join(viper.GetString("WriteDirectory"), name+".log")
	} else {
		// otherwise, nest within a directory with the same name as the raid
		filepath = path.Join(viper.GetString("WriteDirectory"), name, name+".log")
	}

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		// mkdir all directories from filepath
		os.MkdirAll(path.Dir(filepath), os.ModePerm)
		os.Create(filepath)
	}

	logFile, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)

	if err != nil {
		log.Panic(err) // TODO handle this error better
	}
	multi := io.MultiWriter(logFile, os.Stdout)

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.LevelFromString(viper.GetString("loglevel")),
		JSONFormat: jsonFormat,
		Output:     multi,
	})
	log.SetOutput(logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: false, InferLevelsWithTimestamp: false}))
	// log.SetFlags(0)
	return logger
}

// AddStrikeResult adds a StrikeResult to the Tactic
func (r *Tactic) AddStrikeResult(name string, result StrikeResult) {
	if r.StrikeResults == nil {
		r.StrikeResults = make(map[string]StrikeResult)
	}
	r.StrikeResults[name] = result
}

// WriteStrikeResultsJSON unmarhals the Tactic into a JSON file in the user's WriteDirectory
func (r *Tactic) WriteStrikeResultsJSON() error {
	// Log an error if RaidName was not provided
	if r.TacticName == "" {
		return errors.New("RaidName was not provided before attempting to write results")
	}
	filepath := path.Join(viper.GetString("WriteDirectory"), r.TacticName, "results.json")

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		os.MkdirAll(viper.GetString("WriteDirectory"), os.ModePerm)
		os.Create(filepath)
	}

	// Write results to file
	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// Marshal results to JSON
	json, err := json.Marshal(r)
	if err != nil {
		return err
	}

	// Write JSON to file
	_, err = file.Write(json)
	if err != nil {
		return err
	}

	return nil
}

// WriteStrikeResultsYAML unmarhals the Tactic into a YAML file in the user's WriteDirectory
func (r *Tactic) WriteStrikeResultsYAML() error {
	// Log an error if RaidName was not provided
	if r.TacticName == "" {
		panic("RaidName was not provided before attempting to write results")
	}
	filepath := path.Join(viper.GetString("WriteDirectory"), r.TacticName, "results.yaml")

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		os.MkdirAll(viper.GetString("WriteDirectory"), os.ModePerm)
		os.Create(filepath)
	}

	// Write results to file
	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	// Marshal results to YAML
	yaml, err := yaml.Marshal(r)
	if err != nil {
		return err
	}

	// Write YAML to file
	_, err = file.Write(yaml)
	if err != nil {
		return err
	}

	return nil
}
