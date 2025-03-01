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
	Applicability  []string
	Vars           map[string]interface{}
	Error          error
}

func NewConfig(requiredVars []string) Config {
	serviceName := viper.GetString("service") // the currently running service
	topLoglevel := viper.GetString("loglevel")
	topInvasive := viper.GetBool("invasive")
	writeDir := viper.GetString("write-directory")
	write := viper.GetBool("write")
	output := strings.ToLower(strings.TrimSpace(viper.GetString("output")))

	loglevel := viper.GetString(fmt.Sprintf("services.%s.loglevel", serviceName))
	invasive := viper.GetBool(fmt.Sprintf("services.%s.invasive", serviceName))
	applicability := viper.GetStringSlice(fmt.Sprintf("services.%s.applicability", serviceName))
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
	if serviceName != "" && len(applicability) == 0 {
		errString = fmt.Sprintf("no test suites requested for service in config: %s", viper.GetString("config"))
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
		Applicability:  applicability,
		Vars:           vars,
		Error:          err,
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
