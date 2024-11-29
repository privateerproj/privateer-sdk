package config

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

type Config struct {
	ServiceName    string // Must be unique in the config file or logs will be overwritten
	LogLevel       string
	Logger         hclog.Logger
	WriteDirectory string
	Invasive       bool
	Tactics        []string
	Vars           map[string]interface{}
	Error          error
}

func NewConfig(requiredVars []string) Config {
	serviceName := viper.GetString("service") // the currently running service
	topLoglevel := viper.GetString("loglevel")
	topInvasive := viper.GetBool("invasive")
	writeDir := viper.GetString("write-directory")

	loglevel := viper.GetString(fmt.Sprintf("services.%s.loglevel", serviceName))
	invasive := viper.GetBool(fmt.Sprintf("services.%s.invasive", serviceName))
	tactics := viper.GetStringSlice(fmt.Sprintf("services.%s.tactics", serviceName))
	vars := viper.GetStringMap(fmt.Sprintf("services.%s.vars", serviceName))

	if loglevel == "" && topLoglevel != "" {
		loglevel = topLoglevel
	} else if loglevel == "" {
		loglevel = "Error"
	}

	if !invasive && topInvasive {
		invasive = topInvasive
	}

	if writeDir == "" {
		writeDir = defaultWritePath()
	}

	var errString string
	if serviceName != "" && len(tactics) == 0 {
		errString = fmt.Sprintf("no tactics requested for service in config: %s", viper.GetString("config"))
	}

	var missingVars []string
	for _, v := range requiredVars {
		found := viper.Get(fmt.Sprintf("services.%s.vars.%s", serviceName, v))
		if found == nil || found == "" {
			missingVars = append(missingVars, v)
		}
	}
	if len(missingVars) > 0 {
		errString = fmt.Sprintf("missing required variables: %v", missingVars)
	}

	var err error
	if errString != "" {
		err = errors.New(errString)
	}

	config := Config{
		ServiceName:    serviceName,
		LogLevel:       loglevel,
		WriteDirectory: writeDir,
		Invasive:       invasive,
		Tactics:        tactics,
		Vars:           vars,
		Error:          err,
	}
	config.SetConfig(serviceName, false)
	config.Logger.Trace(fmt.Sprintf("Creating a new config instance for service '%s'%v", serviceName, config))
	config.Logger.Trace(fmt.Sprintf("loglevel: %s", loglevel))
	config.Logger.Trace(fmt.Sprintf("write-directory: %v", invasive))
	config.Logger.Trace(fmt.Sprintf("invasive: %v", writeDir))
	config.Logger.Trace(fmt.Sprintf("tactics: %v", tactics))
	config.Logger.Trace(fmt.Sprintf("vars: %v", vars))
	return config
}

func defaultWritePath() string {
	home, err := os.UserHomeDir()
	datetime := time.Now().Local().Format(time.RFC3339)
	dirName := strings.Replace(datetime, ":", "", -1)
	if err != nil {
		return ""
	}
	return filepath.Join(home, "privateer", "logs", dirName)
}

func (c *Config) SetConfig(name string, jsonFormat bool) {
	var logFilePath string
	logFile := name + ".log"
	if name == "overview" {
		// if this is not a raid, do not nest within a directory
		logFilePath = path.Join(c.WriteDirectory, logFile)
	} else {
		// otherwise, nest within a directory with the same name as the raid
		logFilePath = path.Join(c.WriteDirectory, name, logFile)
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
		Level:      hclog.LevelFromString(c.LogLevel),
		JSONFormat: jsonFormat,
		Output:     multi,
	})
	log.SetOutput(logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: false, InferLevelsWithTimestamp: false}))
	c.Logger = logger
}
