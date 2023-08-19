package logging

import (
	"io"
	"log"

	hclog "github.com/hashicorp/go-hclog"
)

var (
	activeLogger hclog.Logger
	loggers      map[string]hclog.Logger // map of loggers by name
)

func init() {
	loggers = make(map[string]hclog.Logger)
}

// Logger returns the active logger for use in
// statements such as Logger().Info("")
func Logger() hclog.Logger {
	return activeLogger
}

// GetLogger returns an hc logger with the provided name.
// It will creates or update the logger to use the provided level and format.
// If the logger already exists, it will return the existing logger.
// For level options, reference:
// https://github.com/hashicorp/go-hclog/blob/master/logger.go#L19
func GetLogger(name string, level string, jsonFormat bool) hclog.Logger {
	if loggers[name] == nil {
		hcLevel := hclog.LevelFromString(level)
		loggers[name] = hclog.New(&hclog.LoggerOptions{
			Level:      hcLevel,
			JSONFormat: jsonFormat,
		})
	}
	logger := loggers[name]
	writer := logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true})
	SetLogWriter(writer)
	return logger
}

// SetLogWriter modifies the output for the standard logger.
// This aligns the functionality of the hclog logger with the standard logger.
func SetLogWriter(logWriter io.Writer) {
	log.SetFlags(0)
	log.SetOutput(logWriter)
}
