package pluginkit

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/shared"
)

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name         string
		orchestrator *EvaluationOrchestrator
		err          error
		want         int
	}{
		{
			name:         "nil error, nil orchestrator returns TestPass",
			orchestrator: nil,
			err:          nil,
			want:         shared.TestPass,
		},
		{
			name:         "nil error, no suites returns TestPass",
			orchestrator: &EvaluationOrchestrator{},
			err:          nil,
			want:         shared.TestPass,
		},
		{
			name: "nil error, all suites passed returns TestPass",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.Passed},
					{Result: gemara.Passed},
				},
			},
			err:  nil,
			want: shared.TestPass,
		},
		{
			name: "nil error, NotRun and NotApplicable count as pass",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.NotRun},
					{Result: gemara.NotApplicable},
					{Result: gemara.Passed},
				},
			},
			err:  nil,
			want: shared.TestPass,
		},
		{
			name: "nil error, one suite Failed returns TestFail",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.Passed},
					{Result: gemara.Failed},
				},
			},
			err:  nil,
			want: shared.TestFail,
		},
		{
			name: "nil error, NeedsReview returns TestFail",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.NeedsReview},
				},
			},
			err:  nil,
			want: shared.TestFail,
		},
		{
			name: "nil error, Unknown returns TestFail",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.Unknown},
				},
			},
			err:  nil,
			want: shared.TestFail,
		},
		{
			name:         "BAD_LOADER returns InternalError",
			orchestrator: &EvaluationOrchestrator{},
			err:          BAD_LOADER("svc", errors.New("connection refused"), "mob30"),
			want:         shared.InternalError,
		},
		{
			name:         "BAD_CONFIG returns InternalError",
			orchestrator: &EvaluationOrchestrator{},
			err:          BAD_CONFIG(errors.New("missing field"), "mob10"),
			want:         shared.InternalError,
		},
		{
			name:         "WRITE_FAILED returns InternalError",
			orchestrator: &EvaluationOrchestrator{},
			err:          WRITE_FAILED("svc", "disk full", "wr40"),
			want:         shared.InternalError,
		},
		{
			name:         "CORRUPTION_FOUND returns InternalError",
			orchestrator: &EvaluationOrchestrator{},
			err:          CORRUPTION_FOUND("ev40"),
			want:         shared.InternalError,
		},
		{
			name:         "NO_EVALUATION_SUITES returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          NO_EVALUATION_SUITES("mob50"),
			want:         shared.BadUsage,
		},
		{
			name:         "BAD_CATALOG returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          BAD_CATALOG("svc", "no controls", "aos20"),
			want:         shared.BadUsage,
		},
		{
			name:         "CONFIG_NOT_INITIALIZED returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          CONFIG_NOT_INITIALIZED("ev10"),
			want:         shared.BadUsage,
		},
		{
			name:         "NO_ASSESSMENT_STEPS_PROVIDED returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          NO_ASSESSMENT_STEPS_PROVIDED("sel10"),
			want:         shared.BadUsage,
		},
		{
			name:         "EVAL_SUITE_CRASHED returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          EVAL_SUITE_CRASHED("sel20"),
			want:         shared.BadUsage,
		},
		{
			name:         "EVALUATION_ORCHESTRATOR_NAMES_NOT_SET returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          EVALUATION_ORCHESTRATOR_NAMES_NOT_SET("svc", "plg", "mob40"),
			want:         shared.BadUsage,
		},
		{
			name:         "NO_MATCHING_CATALOGS returns BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          NO_MATCHING_CATALOGS([]string{"a"}, []string{"b"}, "mob60"),
			want:         shared.BadUsage,
		},
		{
			name:         "BAD_EVAL_LOG wrapping NO_ASSESSMENT_STEPS still classifies as BadUsage",
			orchestrator: &EvaluationOrchestrator{},
			err:          BAD_EVAL_LOG(NO_ASSESSMENT_STEPS_PROVIDED("sel10"), "ev30"),
			want:         shared.BadUsage,
		},
		{
			name:         "unclassified error returns InternalError as safe default",
			orchestrator: &EvaluationOrchestrator{},
			err:          errors.New("something exploded"),
			want:         shared.InternalError,
		},
		{
			name: "error wins over suite results",
			orchestrator: &EvaluationOrchestrator{
				Evaluation_Suites: []*EvaluationSuite{
					{Result: gemara.Failed},
				},
			},
			err:  BAD_LOADER("svc", errors.New("nope"), "mob30"),
			want: shared.InternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExitCodeFor(tt.orchestrator, tt.err)
			if got != tt.want {
				t.Errorf("ExitCodeFor = %d, want %d (err=%v)", got, tt.want, tt.err)
			}
		})
	}
}

// Plugin authors will wrap pluginkit errors with their own context; the
// category sentinel must survive an extra fmt.Errorf("%w") layer.
func TestErrorSentinelsSurviveWrapping(t *testing.T) {
	runtime := fmt.Errorf("ctx: %w", BAD_LOADER("svc", errors.New("x"), "m"))
	if !errors.Is(runtime, ErrRuntime) {
		t.Errorf("wrapped BAD_LOADER did not satisfy errors.Is(ErrRuntime)")
	}
	devBug := fmt.Errorf("ctx: %w", NO_EVALUATION_SUITES("m"))
	if !errors.Is(devBug, ErrDevBug) {
		t.Errorf("wrapped NO_EVALUATION_SUITES did not satisfy errors.Is(ErrDevBug)")
	}
}
