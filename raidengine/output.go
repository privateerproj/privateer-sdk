package raidengine

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path"

	hclog "github.com/hashicorp/go-hclog"
	yaml "gopkg.in/yaml.v3"
)

// GetLogger returns an hc logger with the provided name.
// It will creates or update the logger to use the provided level and format.
// If the logger already exists, it will return the existing logger.
// For level options, reference:
// https://github.com/hashicorp/go-hclog/blob/master/logger.go#L19
func GetLogger(name string, jsonFormat bool) hclog.Logger {
	// Initialize file writer for MultiWriter
	var logFilePath string

	logFile := name + ".log"
	if name == "overview" {
		// if this is not a raid, do not nest within a directory
		logFilePath = path.Join(globalConfig.WriteDirectory, logFile)
	} else {
		// otherwise, nest within a directory with the same name as the raid
		logFilePath = path.Join(globalConfig.WriteDirectory, name, logFile)
	}

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		// mkdir all directories from filepath
		os.MkdirAll(path.Dir(logFilePath), os.ModePerm)
		os.Create(logFilePath)
	}

	logFileObj, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)

	if err != nil {
		log.Panic(err) // TODO: handle this error better
	}
	multi := io.MultiWriter(logFileObj, os.Stdout)

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.LevelFromString(globalConfig.LogLevel),
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
		return errors.New("Tactic name was not provided before attempting to write results")
	}
	filepath := path.Join(globalConfig.WriteDirectory, r.TacticName, "results.json")

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		os.MkdirAll(globalConfig.WriteDirectory, os.ModePerm)
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
		panic("Tactic name was not provided before attempting to write results")
	}
	filepath := path.Join(globalConfig.WriteDirectory, r.TacticName, "results.yaml")

	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		os.MkdirAll(globalConfig.WriteDirectory, os.ModePerm)
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
