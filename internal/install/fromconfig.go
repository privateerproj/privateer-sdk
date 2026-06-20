package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// maxConcurrentInstalls bounds how many plugins FromConfig pulls + verifies at
// once. Installs are network- and verification-bound, so a small pool gives a
// real wall-clock win in CI without hammering the registry with unbounded
// parallel pulls.
const maxConcurrentInstalls = 4

// FromConfig installs every plugin referenced by the active config that is not
// already present in the local manifest. Each service's plugin field is the
// grc.store <namespace>/<plugin_id> coordinate (optionally pinned with an
// @<version>); missing plugins are resolved against grc.store and installed
// concurrently, bounded by maxConcurrentInstalls. The first failure cancels the
// remaining installs and is returned wrapped with its coordinate.
//
// It is a no-op (returns nil) when no services are configured or every requested
// plugin is already installed. Per-plugin progress is buffered and flushed to w
// as a single block per plugin, so concurrent installs never interleave their
// output mid-line; the caller owns flushing w.
//
// It reads the active config from the same viper state as the config getters, so
// the caller must have loaded config (e.g. the CLI's PersistentPreRun) before
// invoking it — otherwise no services are visible and it is a no-op.
func FromConfig(ctx context.Context, w io.Writer) error {
	args, err := missingFromConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}

	// Each install writes to its own buffer; completed buffers are flushed to w
	// under flushMu so a plugin's progress lines stay together as one block
	// instead of interleaving with another install still in flight.
	bufs := make([]bytes.Buffer, len(args))
	var flushMu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentInstalls)
	for i, arg := range args {
		g.Go(func() error {
			err := FromStore(ctx, &bufs[i], arg)
			flushMu.Lock()
			_, _ = io.Copy(w, &bufs[i])
			flushMu.Unlock()
			if err != nil {
				return fmt.Errorf("installing %s: %w", arg, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// missingFromConfig resolves the active config's services to the deduped list of
// <namespace>/<plugin_id>[@<version>] coordinates that are not yet installed.
func missingFromConfig() ([]string, error) {
	services := config.GetServices()
	if len(services) == 0 {
		return nil, nil
	}

	m, err := manifest.Load(config.GetBinariesPath())
	if err != nil {
		return nil, fmt.Errorf("loading plugin manifest: %w", err)
	}

	// Dedupe by name@version so two services pinning the same plugin+version
	// install it once (mirrors the run-time resolver's dedupe key).
	seen := make(map[string]bool)
	var args []string
	for serviceName := range services {
		name := config.GetServicePlugin(serviceName)
		if name == "" {
			continue
		}
		version := config.GetServiceVersion(serviceName)
		dedupKey := name + "@" + version
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		if installedInManifest(m, name, version) {
			continue
		}

		// The service's plugin field IS the grc.store coordinate FromStore
		// expects; append the @version pin when one is set.
		arg := name
		if version != "" {
			arg = name + "@" + version
		}
		args = append(args, arg)
	}
	return args, nil
}

// installedInManifest reports whether the requested plugin is already present:
// an exact name+version match when a version is pinned, else any installed
// version of the plugin (matching the run-time resolver's "latest installed").
func installedInManifest(m *manifest.Manifest, name, version string) bool {
	if version != "" {
		return m.FindVersion(name, version) != nil
	}
	return m.Latest(name) != nil
}
