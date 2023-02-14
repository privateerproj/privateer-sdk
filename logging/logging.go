package logging

import (
	"io"
	"log"
	"os"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

var (
	activeLogger hclog.Logger
	loggers      map[string]hclog.Logger
)

func init() {
	// Initialize default logger
	name := "default"
	loggers = make(map[string]hclog.Logger)
	UseLogger(name)
}

// Logger returns the active logger for use in
// statements such as Logger().Info("")
func Logger() hclog.Logger {
	return activeLogger
}

// Writer ...
func Writer() io.Writer {
	return activeLogger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true})
}

// newLogger creates a new hclog.Logger instance with the given name and log level
func newLogger(writer io.Writer, jsonFormat bool) hclog.Logger {
	// For level options, reference:
	// https://github.com/hashicorp/go-hclog/blob/master/logger.go#L19
	return hclog.New(&hclog.LoggerOptions{
		Level:      hclog.LevelFromString(viper.GetString("loglevel")),
		Output:     writer,
		JSONFormat: jsonFormat, // TODO: Check env var to determine json format
	})
}

// GetLogger returns an hc logger with the provided name
// and creates or updates an existing logger using args
// args[0] = Level
// args[1] = Output
// args[2] = JSONFormat
func GetLogger(name string, args ...interface{}) hclog.Logger {
	if loggers[name] == nil {
		if len(args) > 2 {
			loggers[name] = newLogger(args[1].(io.Writer), args[2].(bool))
		} else if len(args) > 1 {
			// If json format is not specified, default to false
			loggers[name] = newLogger(args[1].(io.Writer), false)
		} else {
			loggers[name] = newLogger(os.Stderr, false)
		}
	}

	if len(args) > 0 && args[0] != nil {
		// Set loglevel; Defaults to value in config vars
		loggers[name].SetLevel(hclog.LevelFromString(args[0].(string)))
	}
	return loggers[name]
}

// UseLogger creates or retrieves a logger by name
// and sets it as the primary logger for the log package
func UseLogger(name string, args ...interface{}) hclog.Logger {
	logger := GetLogger(name, args...)
	writer := logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true})
	SetLogWriter(name, writer)
	return logger
}

// SetLogWriter sets the log package to use the provided writer
// If the logWriter was not created using this package,
// Logger() will return nil until UseLogger is run
func SetLogWriter(name string, logWriter io.Writer) {
	activeLogger = loggers[name]
	if activeLogger == nil {
		GetLogger(name)
	}
	log.SetFlags(0)
	log.SetOutput(logWriter)
}
