// Package pluginkit provides the core plugin kit functionality for building Privateer plugins.
//
// Each error factory wraps one of the two category sentinels below so
// ExitCodeFor can classify it into the right exit code via errors.Is.
package pluginkit

import (
	"errors"
	"fmt"
)

// ErrRuntime classifies failures during plugin execution that aren't the
// plugin author's fault (loader, write, RPC, corruption). Maps to InternalError.
var ErrRuntime = errors.New("privateer plugin runtime error")

// ErrDevBug classifies SDK misuse or malformed plugin data (missing catalogs,
// unset names, missing assessment steps). Maps to BadUsage.
var ErrDevBug = errors.New("privateer plugin development error")

// Error functions that require no parameters.
var (
	CORRUPTION_FOUND = func(mod string) error {
		return wrap(ErrRuntime, "target state may be corrupted! Halting to prevent futher damage. See logs for more information", mod)
	}
	NO_EVALUATION_SUITES = func(mod string) error {
		return wrap(ErrDevBug, "no control evaluations provided by the plugin", mod)
	}
	EVAL_NAME_MISSING = func(mod string) error {
		return wrap(ErrDevBug, "evaluationSuite name must not be empty", mod)
	}
	CONFIG_NOT_INITIALIZED = func(mod string) error {
		return wrap(ErrDevBug, "configuration not initialized", mod)
	}
	NO_ASSESSMENT_STEPS_PROVIDED = func(mod string) error {
		return wrap(ErrDevBug, "assessment steps not provided", mod)
	}
	NO_ASSESSMENT_REQS_PROVIDED = func(mod string) error {
		return wrap(ErrDevBug, "assessment requirements not provided", mod)
	}
	EVAL_SUITE_CRASHED = func(mod string) error {
		return wrap(ErrDevBug, "evaluation suite crashed", mod)
	}
)

// Error functions that require parameters.
var (
	EVALUATION_ORCHESTRATOR_NAMES_NOT_SET = func(serviceName, pluginName string, mod string) error {
		return wrap(ErrDevBug, fmt.Sprintf("expected service and plugin names to be set. ServiceName='%s' PluginName='%s'", serviceName, pluginName), mod)
	}
	WRITE_FAILED = func(name, err string, mod string) error {
		return wrap(ErrRuntime, fmt.Sprintf("failed to write results for evaluation suite. name: %s, error: %s", name, err), mod)
	}
	BAD_LOADER = func(pluginName string, err error, mod string) error {
		return wrap(ErrRuntime, fmt.Sprintf("failed to load payload for %s: %s", pluginName, err), mod)
	}
	BAD_CATALOG = func(pluginName string, errMsg string, mod string) error {
		return wrap(ErrDevBug, fmt.Sprintf("malformed data in catalog for %s: %s", pluginName, errMsg), mod)
	}
	BAD_EVAL_LOG = func(err error, mod string) error {
		return wrap(ErrDevBug, fmt.Sprintf("failed to setup evaluation log: %s", err), mod)
	}
	BAD_ASSESSMENT_REQS = func(err error, mod string) error {
		return wrap(ErrDevBug, fmt.Sprintf("failed to load assessment requirements from catalog: %s", err), mod)
	}
	BAD_CONFIG = func(err error, mod string) error {
		return wrap(ErrRuntime, fmt.Sprintf("failed to setup config: %s", err), mod)
	}
	NO_MATCHING_CATALOGS = func(requested []string, available []string, mod string) error {
		return wrap(ErrDevBug, fmt.Sprintf("no requested catalogs matched available suites. requested=%v available=%v", requested, available), mod)
	}
)

// wrap chains the category sentinel via %w so errors.Is works, while keeping
// the legacy "msg+mod" prefix that existing tests substring-match against.
func wrap(sentinel error, msg, mod string) error {
	return fmt.Errorf("%s+%s: %w", msg, mod, sentinel)
}

func errMod(err any, mod string) error {
	if err != nil {
		return fmt.Errorf("%+v+%s", err, mod)
	}
	return nil
}
