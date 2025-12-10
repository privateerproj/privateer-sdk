// Package pluginkit provides the core plugin kit functionality for building Privateer plugins.
//
// This file contains error definitions to streamline testing and log management.
package pluginkit

import (
	"fmt"
)

// Error functions that require no parameters.
var (
	CORRUPTION_FOUND = func(mod string) error {
		return errMod("target state may be corrupted! Halting to prevent futher damage. See logs for more information", mod)
	}
	NO_EVALUATION_SUITES = func(mod string) error {
		return errMod("no control evaluations provided by the plugin", mod)
	}
	EVAL_NAME_MISSING = func(mod string) error {
		return errMod("evaluationSuite name must not be empty", mod)
	}
	CONFIG_NOT_INITIALIZED = func(mod string) error {
		return errMod("configuration not initialized", mod)
	}
	NO_ASSESSMENT_STEPS_PROVIDED = func(mod string) error {
		return errMod("assessment steps not provided", mod)
	}
	NO_ASSESSMENT_REQS_PROVIDED = func(mod string) error {
		return errMod("assessment requirements not provided", mod)
	}
	EVAL_SUITE_CRASHED = func(mod string) error {
		return errMod("evaluation suite crashed", mod)
	}
)

// Error functions that require parameters.
var (
	EVALUATION_ORCHESTRATOR_NAMES_NOT_SET = func(serviceName, pluginName string, mod string) error {
		return errMod(fmt.Errorf("expected service and plugin names to be set. ServiceName='%s' PluginName='%s'", serviceName, pluginName), mod)
	}
	WRITE_FAILED = func(name, err string, mod string) error {
		return errMod(fmt.Errorf("failed to write results for evaluation suite. name: %s, error: %s", name, err), mod)
	}
	BAD_LOADER = func(pluginName string, err error, mod string) error {
		return errMod(fmt.Errorf("failed to load payload for %s: %s", pluginName, err), mod)
	}
	BAD_CATALOG = func(pluginName string, errMsg string, mod string) error {
		return errMod(fmt.Errorf("malformed data in catalog for %s: %s", pluginName, errMsg), mod)
	}
	BAD_EVAL_LOG = func(err error, mod string) error {
		return errMod(fmt.Errorf("failed to setup evaluation log: %w", err), mod)
	}
	BAD_ASSESSMENT_REQS = func(err error, mod string) error {
		return errMod(fmt.Errorf("failed to load assessment requirements from catalog: %w", err), mod)
	}
	BAD_CONFIG = func(err error, mod string) error {
		return errMod(fmt.Errorf("failed to setup config: %w", err), mod)
	}
)

func errMod(err any, mod string) error {
	if err != nil {
		return fmt.Errorf("%+v+%s", err, mod)
	}
	return nil
}
