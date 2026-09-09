// Package config provides configuration management for Privateer plugins.
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/privateerproj/privateer-sdk/utils"
	"github.com/spf13/viper"
)

const defaultServiceName = "overview"

const aiAPIKeyConfigWarning = "ai_api_key is set in configuration; avoid committing credentials and prefer PVTR_AI_API_KEY or ai_api_key_env when possible"

var allowedOutputTypes = []string{"json", "yaml", "sarif", "gemara"}

var inheritedTopLevelVarKeys = []string{
	"ai_provider",
	"ai_model",
	"ai_api_key",
	"ai_api_key_env",
	"ai_base_url",
	"ai_timeout",
	"ai_max_tokens",
	"ai_skip",
}

// Config holds the configuration for a plugin execution.
type Config struct {
	ServiceName    string // Must be unique in the config file or logs will be overwritten
	LogLevel       string
	Logger         hclog.Logger
	Write          bool
	Output         string
	IncludePayload bool
	WriteDirectory string
	Invasive       bool
	Policy         Policy
	Vars           map[string]interface{}
	Error          error

	Benchmark            bool
	BenchmarkPayloadOnly bool
}

// Policy defines the control catalogs and applicability settings for a plugin.
// TODO: We will want to replace this with a Gemara layer3 object now that those are ready.
type Policy struct {
	ControlCatalogs []string
	Applicability   []string
}

// NewConfig creates a new Config instance from viper configuration.
func NewConfig(requiredVars []string) Config {
	var errString string

	serviceName := TargetName() // the currently running target; if empty, we're probably running from core
	svcKey := targetsKey()      // "targets" or its legacy "services" alias, whichever the config uses
	fileConfig, fileConfigErr := readConfigFileSnapshot()
	aiAPIKeyInConfigFile := hasConfigFileAIAPIKey(fileConfig, serviceName, svcKey)

	write := viper.GetBool("write")                                         // defaults to true, but allow the user to disable file writing
	output := strings.ToLower(strings.TrimSpace(viper.GetString("output"))) // defaults to yaml; can be set to json, sarif, or gemara
	includePayload := viper.GetBool("include-payload")                      // defaults to false; payload is omitted unless explicitly requested
	benchmark := viper.GetBool("benchmark")                                 // defaults to false
	benchmarkPayloadOnly := viper.GetBool("benchmark-payload-only")         // defaults to false; loader only, skip steps

	vars := viper.GetStringMap("vars")
	localVars := viper.GetStringMap(fmt.Sprintf("%s.%s.vars", svcKey, serviceName))
	for key, value := range localVars {
		// Overwrite or add local vars onto the global vars
		vars[key] = value
	}
	// AI settings are materialized into Vars so SDK consumers see one resolved
	// view. Environment values outrank target and top-level file values.
	for _, key := range inheritedTopLevelVarKeys {
		if key == "ai_skip" {
			if value, found := resolvedAISkip(fileConfig, vars); found {
				vars[key] = value
			}
			continue
		}
		if key != "ai_api_key" && key != "ai_api_key_env" {
			if value, found := aiEnvironmentValue(key); found {
				vars[key] = value
				continue
			}
		}
		if _, exists := vars[key]; exists {
			continue
		}
		if value, found := topLevelConfigValue(fileConfig, key); found {
			vars[key] = value
		}
	}

	topLoglevel := viper.GetString("loglevel")
	loglevel := viper.GetString(fmt.Sprintf("%s.%s.loglevel", svcKey, serviceName))
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
	invasive := viper.GetBool(fmt.Sprintf("%s.%s.invasive", svcKey, serviceName))
	if !invasive && topInvasive {
		invasive = topInvasive
	}

	topCatalogs := viper.GetStringSlice("policy.catalogs")
	catalogs := viper.GetStringSlice(fmt.Sprintf("%s.%s.policy.catalogs", svcKey, serviceName))
	if len(catalogs) == 0 {
		catalogs = topCatalogs
	}

	topApplicability := viper.GetStringSlice("policy.applicability")
	applicability := viper.GetStringSlice(fmt.Sprintf("%s.%s.policy.applicability", svcKey, serviceName))
	if len(applicability) == 0 {
		applicability = topApplicability
	}

	if serviceName != "" && (len(applicability) == 0 || len(catalogs) == 0) {
		errString = fmt.Sprintf("invalid policy for service %s. applicability=%v catalogs=%v",
			serviceName, len(applicability), len(catalogs))
		if svcKey == "targets" && viper.IsSet("services."+serviceName) {
			errString += fmt.Sprintf("; %q is defined under the legacy services key, which is ignored because a targets key is present", serviceName)
		}
	}

	var missingVars []string
	for _, key := range requiredVars {
		if _, ok := vars[key]; !ok {
			missingVars = append(missingVars, key)
		}
	}
	if len(missingVars) > 0 {
		errString = fmt.Sprintf("missing required variables: %v", missingVars)
	}

	if output == "" {
		output = "yaml"
	} else if ok := slices.Contains(allowedOutputTypes, output); !ok {
		errString = "bad output type, allowed output types are json, yaml, sarif, or gemara"
	}

	var err error
	if errString != "" {
		err = errors.New(errString)
	}
	err = errors.Join(err, fileConfigErr)

	config := Config{
		ServiceName:          serviceName,
		LogLevel:             loglevel,
		WriteDirectory:       writeDir,
		Write:                write,
		Output:               output,
		IncludePayload:       includePayload,
		Invasive:             invasive,
		Benchmark:            benchmark,
		BenchmarkPayloadOnly: benchmarkPayloadOnly,
		Policy: Policy{
			ControlCatalogs: catalogs,
			Applicability:   applicability,
		},
		Vars:  vars,
		Error: err,
	}
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	config.SetupLogging(serviceName, output == "json")
	if aiAPIKeyInConfigFile {
		config.Logger.Warn(aiAPIKeyConfigWarning)
	}
	printSanitizedVars(config.Logger, vars)
	config.Logger.Trace("Creating a new config instance for service",
		"serviceName", serviceName,
		"loglevel", loglevel,
		"write", write,
		"write-directory", writeDir,
		"invasive", invasive,
		"applicability", applicability,
		"control-catalogs", catalogs,
		"output", output,
	)
	return config
}

func readConfigFileSnapshot() (*viper.Viper, error) {
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		return nil, nil
	}
	fileConfig := viper.New()
	fileConfig.SetConfigFile(configFile)
	if err := fileConfig.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read raw config file: %w", err)
	}
	return fileConfig, nil
}

func configFileValue(fileConfig *viper.Viper, key string) (interface{}, bool) {
	if fileConfig != nil {
		if !fileConfig.InConfig(key) {
			return nil, false
		}
		return fileConfig.Get(key), true
	}
	if !viper.InConfig(key) {
		return nil, false
	}
	return viper.Get(key), true
}

func hasConfigFileAIAPIKey(fileConfig *viper.Viper, serviceName, svcKey string) bool {
	for _, key := range []string{"ai_api_key", "vars.ai_api_key"} {
		if value, found := configFileValue(fileConfig, key); found {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	if serviceName != "" {
		key := fmt.Sprintf("%s.%s.vars.ai_api_key", svcKey, serviceName)
		if value, found := configFileValue(fileConfig, key); found {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func resolvedAISkip(fileConfig *viper.Viper, vars map[string]interface{}) (interface{}, bool) {
	candidates := make([]interface{}, 0, 3)
	if value, found := aiEnvironmentValue("ai_skip"); found {
		candidates = append(candidates, value)
	}
	if value, found := vars["ai_skip"]; found {
		candidates = append(candidates, value)
	}
	if value, found := topLevelConfigValue(fileConfig, "ai_skip"); found {
		candidates = append(candidates, value)
	}
	for _, value := range candidates {
		if skip, ok := value.(bool); ok && skip {
			return true, true
		}
	}
	for _, value := range candidates {
		if _, ok := value.(bool); !ok {
			return value, true
		}
	}
	if len(candidates) > 0 {
		return false, true
	}
	return nil, false
}

func aiEnvironmentValue(key string) (interface{}, bool) {
	text, found := os.LookupEnv("PVTR_" + strings.ToUpper(key))
	text = strings.TrimSpace(text)
	if !found || text == "" {
		return nil, false
	}
	switch key {
	case "ai_max_tokens":
		value, err := strconv.Atoi(text)
		if err == nil {
			return value, true
		}
	case "ai_skip":
		value, err := strconv.ParseBool(text)
		if err == nil {
			return value, true
		}
	}
	return text, true
}

func topLevelConfigValue(fileConfig *viper.Viper, key string) (interface{}, bool) {
	if value, found := configFileValue(fileConfig, key); found {
		return value, true
	}
	if viper.InConfig(key) || viper.IsSet(key) {
		if key == "ai_api_key_env" {
			if _, unsupported := os.LookupEnv("PVTR_AI_API_KEY_ENV"); unsupported && !viper.InConfig(key) {
				return nil, false
			}
		}
		return viper.Get(key), true
	}
	return nil, false
}

func printSanitizedVars(logger hclog.Logger, vars map[string]interface{}) {
	sanitizedVars := sanitizeVars(vars)
	logger.Trace("Using vars", "vars", sanitizedVars)
}

func sanitizeVars(vars map[string]interface{}) map[string]interface{} {
	sensitivePatterns := []string{"token", "auth", "password", "secret", "apikey", "api_key"}
	sanitizedVars := make(map[string]interface{})
	for key, value := range vars {
		redact := false
		lower := strings.ToLower(key)
		// ai_api_key_env holds the name of an environment variable, not a credential.
		if lower != "ai_api_key_env" {
			for _, pattern := range sensitivePatterns {
				if strings.Contains(lower, pattern) {
					redact = true
					break
				}
			}
		}
		if redact {
			sanitizedVars[key] = "REDACTED"
		} else {
			sanitizedVars[key] = value
		}
	}
	return sanitizedVars
}

// defaultWritePath returns the default write directory, computed once per
// process so that every config created during a run shares the same
// timestamped log directory.
var defaultWritePath = sync.OnceValue(func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	datetime := time.Now().Local().Format(time.RFC3339)
	dirName := strings.ReplaceAll(datetime, ":", "")
	return filepath.Join(home, ".privateer", "logs", dirName)
})

// SetupLogging configures logging for the plugin with the given name and format.
func (c *Config) SetupLogging(name string, jsonFormat bool) {
	var logFilePath string
	logFile := name + ".log"
	if name == defaultServiceName {
		// if this is not a plugin, do not nest within a directory
		logFilePath = path.Join(c.WriteDirectory, logFile)
	} else {
		// otherwise, nest within a directory with the same name as the plugin
		logFilePath = path.Join(c.WriteDirectory, name, logFile)
	}

	writer := io.Writer(os.Stderr)
	if c.Write && name != defaultServiceName {
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
		_ = os.MkdirAll(path.Dir(logFilePath), utils.DirPermissions)
		_, _ = os.Create(logFilePath)
	}

	logFileObj, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)

	if err != nil {
		log.Panic(err) // TODO: handle this error better
	}

	writer := io.MultiWriter(logFileObj, os.Stderr)
	return writer
}
