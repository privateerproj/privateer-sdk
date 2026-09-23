package install

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/privateerproj/privateer-sdk/pluginkit"
)

// manifestExecTimeout bounds running the plugin's publish-manifest subcommand.
// It is the user's own freshly-built binary, so this only guards against a
// hung process, not a hostile one.
const manifestExecTimeout = 30 * time.Second

// RunPublishManifest runs the plugin binary's publish-manifest subcommand and
// decodes its JSON stdout. Only ever run on the user's own build (publish, or
// a --local install); install never executes bytes pulled from a registry.
func RunPublishManifest(ctx context.Context, hostBinaryPath string) (pluginkit.PublishManifest, error) {
	var zero pluginkit.PublishManifest
	ctx, cancel := context.WithTimeout(ctx, manifestExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hostBinaryPath, pluginkit.PublishManifestCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return zero, fmt.Errorf("%s %s: %w: %s", filepath.Base(hostBinaryPath), pluginkit.PublishManifestCommand, err, detail)
		}
		return zero, fmt.Errorf("%s %s: %w", filepath.Base(hostBinaryPath), pluginkit.PublishManifestCommand, err)
	}
	var m pluginkit.PublishManifest
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &m); err != nil {
		return zero, fmt.Errorf("decoding publish manifest JSON: %w", err)
	}
	return m, nil
}
