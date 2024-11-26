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
	loglevel := viper.GetString("services." + serviceName + ".loglevel")
	topLoglevel := viper.GetString("loglevel")
	invasive := viper.GetBool("services." + serviceName + ".invasive")
	topInvasive := viper.GetBool("invasive")
	writeDir := viper.GetString("write-directory")
	tactics := viper.GetStringSlice("services." + serviceName + ".tactics")
	vars := viper.GetStringMap("services." + serviceName + ".vars")

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
	if len(tactics) == 0 {
		errString = fmt.Sprintf("no tactics specified for service %s", serviceName)
	}

	var missingVars []string
	for _, v := range requiredVars {
		if _, ok := vars[v]; !ok {
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
		Tactics:        viper.GetStringSlice("services." + serviceName + ".tactics"),
		Vars:           vars,
		Error:          err,
	}
	config.SetLogger(serviceName, false)
	config.Logger.Debug(fmt.Sprintf("Creating a new config instance for service '%s'", serviceName))

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

func (c *Config) SetLogger(name string, jsonFormat bool) {
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
