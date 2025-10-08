// errors.go contains error definitions to streamline testing and log management
package pluginkit

import (
	"errors"
	"fmt"
)

// Errors with no parameters
var (
	CORRUPTION_FOUND = func() error {
		return errors.New("target state may be corrupted! Halting to prevent futher damage. See logs for more information")
	}
	NO_EVALUATION_SUITES = func() error {
		return errors.New("no control evaluations provided by the plugin")
	}
	EVAL_NAME_MISSING = func() error {
		return errors.New("evaluationSuite name must not be empty")
	}
	CONFIG_NOT_INITIALIZED = func() error {
		return errors.New("configuration not initialized")
	}
	NO_ASSESSMENT_STEPS_PROVIDED = func() error {
		return errors.New("assessment steps not provided")
	}
)

// Errors with parameters required
var (
	EVALUATION_ORCHESTRATOR_NAMES_NOT_SET = func(serviceName, pluginName string) error {
		return fmt.Errorf("expected service and plugin names to be set. ServiceName='%s' PluginName='%s'", serviceName, pluginName)
	}
	WRITE_FAILED = func(name, err string) error {
		return fmt.Errorf("failed to write results for evaluation suite. name: %s, error: %s", name, err)
	}
	BAD_LOADER = func(pluginName string, err error) error {
		return fmt.Errorf("failed to load payload for %s: %s", pluginName, err)
	}
)
