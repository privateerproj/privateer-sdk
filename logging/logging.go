package logging

import (
	"io"
	"log"

	hclog "github.com/hashicorp/go-hclog"
)

var (
	activeLogger hclog.Logger
	loggers      map[string]hclog.Logger
)

func init() {
	// Initialize default logger
	// name := "default"
	loggers = make(map[string]hclog.Logger)
	// UseLogger(name)
}

// Logger returns the active logger for use in
// statements such as Logger().Info("")
func Logger() hclog.Logger {
	return activeLogger
}

// GetLogger returns an hc logger with the provided name.
// It will creates or update the logger to use the provided level and format.
func GetLogger(name string, level string, jsonFormat bool) hclog.Logger {
	if loggers[name] == nil {
		// For level options, reference:
		// https://github.com/hashicorp/go-hclog/blob/master/logger.go#L19
		hcLevel := hclog.LevelFromString(level)
		loggers[name] = hclog.New(&hclog.LoggerOptions{
			Level:      hcLevel,
			JSONFormat: jsonFormat,
		})
	}
	logger := loggers[name]
	writer := logger.StandardWriter(&hclog.StandardLoggerOptions{InferLevels: true})
	SetLogWriter(name, writer)
	return logger
}

// SetLogWriter sets the log package to use the provided writer
func SetLogWriter(name string, logWriter io.Writer) {
	log.SetFlags(0)
	log.SetOutput(logWriter)
}
