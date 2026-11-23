package pluginkit

import (
	"errors"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/shared"
)

// ExitCodeFor maps the outcome of EvaluationOrchestrator.Mobilize into a
// canonical privateer exit code. Plugin authors call this from Start:
//
//	func (p *Plugin) Start() (int, error) {
//	    err := p.orchestrator.Mobilize()
//	    return pluginkit.ExitCodeFor(p.orchestrator, err), err
//	}
//
// A non-nil error wrapping ErrRuntime → InternalError; ErrDevBug → BadUsage;
// any other non-nil error → InternalError. With nil error, suite results
// containing Failed/NeedsReview/Unknown → TestFail; every suite NotRun (nothing
// evaluated, e.g. all requirements retired) → NoTests; otherwise TestPass.
func ExitCodeFor(orch *EvaluationOrchestrator, mobilizeErr error) int {
	if mobilizeErr != nil {
		if errors.Is(mobilizeErr, ErrDevBug) {
			return shared.BadUsage
		}
		return shared.InternalError
	}
	if orch == nil {
		return shared.TestPass
	}
	allNotRun := len(orch.Evaluation_Suites) > 0
	for _, suite := range orch.Evaluation_Suites {
		switch suite.Result {
		case gemara.Failed, gemara.NeedsReview, gemara.Unknown:
			return shared.TestFail
		}
		if suite.Result != gemara.NotRun {
			allNotRun = false
		}
	}
	// Every suite ran but nothing in it was evaluated, for example because every
	// requirement is retired. That is not a pass.
	if allNotRun {
		return shared.NoTests
	}
	return shared.TestPass
}
