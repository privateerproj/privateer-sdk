package pluginkit

import (
	"fmt"

	"github.com/gemaraproj/go-gemara"
)

// TypedAssessmentStep is the shape of a payload-typed assessment step: the same
// contract as gemara.AssessmentStep, but receiving a concrete payload type T
// instead of an untyped any.
//
// The type parameter S on the registration helpers below is constrained to
// ~TypedAssessmentStep[T] so a plugin may keep its own named step type (e.g.
// `type TypedStep func(data.Payload) (...)`) and pass its existing step maps
// unchanged.
type TypedAssessmentStep[T any] func(T) (gemara.Result, string, gemara.ConfidenceLevel)

// FuncName resolves a function value's symbol name, as gemara names steps.
// Exported so plugins can record step identity themselves; the typed
// registration helpers below already do this on the caller's behalf.
//
// The name can only be resolved while the function is still a symbol. Once a
// value has been captured into a closure — as happens when a plugin adapts its
// own steps to gemara.AssessmentStep — every closure produced by that literal
// shares one code pointer, and the original name is unrecoverable. That is why
// the adaptation below lives in the SDK: it is the last point at which the
// plugin's function is still nameable.
func FuncName(fn any) string {
	return funcName(fn)
}

// adaptTypedSteps converts payload-typed steps into gemara.AssessmentStep,
// resolving each step's name before the closure captures it. It returns the
// adapted steps and a parallel map of names keyed by requirement id.
func adaptTypedSteps[S ~func(T) (gemara.Result, string, gemara.ConfidenceLevel), T any](
	steps map[string][]S,
) (map[string][]gemara.AssessmentStep, map[string][]string) {
	adapted := make(map[string][]gemara.AssessmentStep, len(steps))
	names := make(map[string][]string, len(steps))
	for id, list := range steps {
		for _, step := range list {
			fn := step // capture per iteration, not the loop variable
			names[id] = append(names[id], FuncName(fn))
			adapted[id] = append(adapted[id], func(payload any) (gemara.Result, string, gemara.ConfidenceLevel) {
				typed, ok := payload.(T)
				if !ok {
					var zero T
					return gemara.Unknown, fmt.Sprintf("expected %T, got %T", zero, payload), 0
				}
				return fn(typed)
			})
		}
	}
	return adapted, names
}

// AddEvaluationSuiteTyped registers an evaluation suite whose steps take a
// concrete payload type T rather than an untyped any. The SDK performs the
// payload type assertion once, so plugins need neither a per-step payload guard
// nor their own adapter — and because the adaptation happens here, each step's
// real function name survives into the benchmark report.
//
// It is a package-level function rather than a method because Go does not allow
// type parameters on methods.
//
// All steps in one call share the payload type T; register a second suite for a
// step family that consumes a different payload.
func AddEvaluationSuiteTyped[S ~func(T) (gemara.Result, string, gemara.ConfidenceLevel), T any](
	v *EvaluationOrchestrator, catalogId string, loader DataLoader, steps map[string][]S,
) error {
	adapted, names := adaptTypedSteps[S, T](steps)
	return v.addEvaluationSuiteNamed(catalogId, loader, adapted, names)
}

// AddEvaluationSuiteTypedForAllCatalogs is AddEvaluationSuiteTyped applied to
// every reference catalog loaded via AddReferenceCatalogs, mirroring
// AddEvaluationSuiteForAllCatalogs.
func AddEvaluationSuiteTypedForAllCatalogs[S ~func(T) (gemara.Result, string, gemara.ConfidenceLevel), T any](
	v *EvaluationOrchestrator, loader DataLoader, steps map[string][]S,
) error {
	if len(v.referenceCatalogs) == 0 {
		return BAD_CATALOG(v.PluginName, "no reference catalogs loaded", "aac10")
	}
	adapted, names := adaptTypedSteps[S, T](steps)
	for catalogId := range v.referenceCatalogs {
		if err := v.addEvaluationSuiteNamed(catalogId, loader, adapted, names); err != nil {
			return err
		}
	}
	return nil
}
