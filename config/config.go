package config

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

var allowedOutputTypes = []string{"json", "yaml"}

type Config struct {
	ServiceName    string // Must be unique in the config file or logs will be overwritten
	LogLevel       string
	Logger         hclog.Logger
	Write          bool
	Output         string
	WriteDirectory string
	Invasive       bool
	Policy         Policy
	Vars           map[string]interface{}
	Error          error
}

type Policy struct {
	// TODO: We will want to replace this with an SCI layer3 object when that is ready
	ControlCatalogs []string
	Applicability   []string
}

func NewConfig(requiredVars []string) Config {
	var errString string

	serviceName := viper.GetString("service") // the currently running service; if empty, we're probably running from core

	write := viper.GetBool("write")                                         // defaults to true, but allow the user to disable file writing
	output := strings.ToLower(strings.TrimSpace(viper.GetString("output"))) // defaults to yaml, but can be set to json

	topVars := viper.GetStringMap("vars")
	vars := viper.GetStringMap(fmt.Sprintf("services.%s.vars", serviceName))
	if len(vars) == 0 {
		vars = topVars
	}

	topLoglevel := viper.GetString("loglevel")
	loglevel := viper.GetString(fmt.Sprintf("services.%s.loglevel", serviceName))
	if loglevel == "" && topLoglevel != "" {
		loglevel = topLoglevel
	} else if loglevel == "" {
		loglevel = "Error"
	}

	writeDir := viper.GetString("write-directory")
	if writeDir == "" {
		writeDir = defaultWritePath()
	}

	topInvasive := viper.GetBool("invasive") // make sure we're actually using this to block changes
	invasive := viper.GetBool(fmt.Sprintf("services.%s.invasive", serviceName))
	if !invasive && topInvasive {
		invasive = topInvasive
	}

	topCatalogs := viper.GetStringSlice("policy.catalogs")
	catalogs := viper.GetStringSlice(fmt.Sprintf("services.%s.policy.catalogs", serviceName))
	if len(catalogs) == 0 {
		catalogs = topCatalogs
	}

	topApplicability := viper.GetStringSlice("policy.applicability")
	applicability := viper.GetStringSlice(fmt.Sprintf("services.%s.policy.applicability", serviceName))
	if len(applicability) == 0 {
		applicability = topApplicability
	}

	if serviceName != "" && (len(applicability) == 0 || len(catalogs) == 0) {
		errString = fmt.Sprintf("invalid policy for service %s. applicability=%v catalogs=%v",
			serviceName, len(applicability), len(catalogs))
	}

	var missingVars []string
	for _, v := range requiredVars {
		found := viper.Get(fmt.Sprintf("services.%s.vars.%s", serviceName, v))
		if found == nil || found == "" {
			foundTop := viper.Get(fmt.Sprintf("vars.%s", v))
			if foundTop == nil || foundTop == "" {
				missingVars = append(missingVars, v)
			}
		}
	}
	if len(missingVars) > 0 {
		errString = fmt.Sprintf("missing required variables: %v", missingVars)
	}

	if output == "" {
		output = "yaml"
	} else if ok := slices.Contains(allowedOutputTypes, output); !ok {
		errString = "bad output type, allowed output types are json or yaml"
	}

	var err error
	if errString != "" {
		err = errors.New(errString)
	}

	config := Config{
		ServiceName:    serviceName,
		LogLevel:       loglevel,
		WriteDirectory: writeDir,
		Write:          write,
		Output:         output,
		Invasive:       invasive,
		Policy: Policy{
			ControlCatalogs: catalogs,
			Applicability:   applicability,
		},
		Vars:  vars,
		Error: err,
	}
	if serviceName == "" {
		serviceName = "overview"
	}
	config.SetupLogging(serviceName, output == "json")
	config.Logger.Trace("Creating a new config instance for service",
		"serviceName", serviceName,
		"loglevel", loglevel,
		"write", write,
		"write-directory", writeDir,
		"invasive", invasive,
		"applicability", applicability,
		"control-catalogs", catalogs,
		"vars", vars,
		"output", output,
	)
	return config
}

func defaultWritePath() string {
	home, err := os.UserHomeDir()
	datetime := time.Now().Local().Format(time.RFC3339)
	dirName := strings.Replace(datetime, ":", "", -1)
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".privateer", "logs", dirName)
}

func (c *Config) SetupLogging(name string, jsonFormat bool) {
	var logFilePath string
	logFile := name + ".log"
	if name == "overview" {
		// if this is not a plugin, do not nest within a directory
		logFilePath = path.Join(c.WriteDirectory, logFile)
	} else {
		// otherwise, nest within a directory with the same name as the plugin
		logFilePath = path.Join(c.WriteDirectory, name, logFile)
	}

	writer := io.Writer(os.Stdout)
	if c.Write {
		writer = c.setupLoggingFilesAndDirectories(logFilePath)
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.LevelFromString(c.LogLevel),
		JSONFormat: jsonFormat,
		Output:     writer,
	})
	log.SetOutput(logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: false, InferLevelsWithTimestamp: false}))
	c.Logger = logger
}

func (c *Config) setupLoggingFilesAndDirectories(logFilePath string) io.Writer {
	// Create log file and directory if it doesn't exist
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		// mkdir all directories from filepath
		_ = os.MkdirAll(path.Dir(logFilePath), os.ModePerm)
		_, _ = os.Create(logFilePath)
	}

	logFileObj, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)

	if err != nil {
		log.Panic(err) // TODO: handle this error better
	}

	writer := io.MultiWriter(logFileObj, os.Stdout)
	return writer
}
