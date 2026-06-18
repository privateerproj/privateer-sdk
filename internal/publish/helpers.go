package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/pluginkit"
)

// manifestExecTimeout bounds running the plugin's publish-manifest subcommand.
// It is the publisher's own freshly-built binary, so this only guards against a
// hung process, not a hostile one.
const manifestExecTimeout = 30 * time.Second

// execPublishManifest selects the host-platform binary from the build and runs
// its publish-manifest subcommand, decoding the JSON stdout. The binary is the
// publisher's own freshly-built plugin, so running it to ask "what do you
// publish as?" is safe (unlike install time, where foreign bytes are never
// executed). stderr is captured only to enrich an error — ReadConfig's
// "[ERROR]" log lands there and is not the manifest.
func execPublishManifest(ctx context.Context, bins []oci.PlatformBinary) (pluginkit.PublishManifest, error) {
	var zero pluginkit.PublishManifest
	host, err := oci.HostPlatformBinary(bins)
	if err != nil {
		return zero, fmt.Errorf("selecting a host binary to run: %w", err)
	}
	hostBinaryPath := host.Path

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

// parseRegistryOverride splits a --registry value that MUST carry a scheme into
// its host and plain-http flag. Requiring the scheme is what lets publish drop a
// separate --plain-http flag: http:// → plain HTTP (local dev), https:// → TLS.
func parseRegistryOverride(raw string) (host string, plainHTTP bool, err error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(raw), "://")
	if !ok {
		return "", false, fmt.Errorf("--registry %q must include a scheme: http://<host> or https://<host>", raw)
	}
	switch scheme {
	case "http":
		plainHTTP = true
	case "https":
		plainHTTP = false
	default:
		return "", false, fmt.Errorf("--registry scheme %q must be http or https", scheme)
	}
	host = strings.TrimRight(rest, "/")
	if host == "" {
		return "", false, fmt.Errorf("--registry %q has no host", raw)
	}
	return host, plainHTTP, nil
}

// uiBaseFromHub derives the web-UI base from the hub's self-reported URL by
// dropping a leading "hub." label (grc.store convention: hub.<env>.grc.store →
// <env>.grc.store / hub.grc.store → grc.store). Best-effort — it's only used to
// point a user at where to claim a namespace; falls back to the hub URL itself.
func uiBaseFromHub(hubURL string) string {
	if hubURL == "" {
		return ""
	}
	scheme, rest, ok := strings.Cut(hubURL, "://")
	if !ok {
		return hubURL
	}
	rest = strings.TrimPrefix(rest, "hub.")
	return scheme + "://" + rest
}
