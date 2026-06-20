package harness

import (
	"context"
	"io"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/install"
)

// ensureRequestedInstalled is the runtime autoinstall preflight that Run runs
// before executing plugins. When the active config opts in via `autoinstall: true`
// (config key "autoinstall", env PVTR_AUTOINSTALL), it installs from grc.store any
// plugin a service requests that is not already present in the local manifest, so
// a single `pvtr run` can install-and-run without a separate `pvtr install` step
// (e.g. a CI job that just runs the tests).
//
// It is a no-op (returns nil) when autoinstall is not enabled — a missing plugin
// then surfaces during the run as the usual "not installed" failure, preserving
// the explicit-install default. The resolve-and-install itself (concurrent,
// first-failure-aborts) is shared with `pvtr install --from-config` via
// install.FromConfig. Per-plugin install progress is written to w; Run flushes w
// before starting plugins.
func ensureRequestedInstalled(ctx context.Context, w io.Writer) error {
	if !config.AutoInstall() {
		return nil
	}
	return install.FromConfig(ctx, w)
}
