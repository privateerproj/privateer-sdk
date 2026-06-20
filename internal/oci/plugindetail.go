package oci

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// PluginDetail is the hub's plugin-level record (GET /v1/plugins/<ns>/<id>) —
// the single source of truth for install resolution: existence, the resolvable
// versions, and the publisher's pinned signer identity. (signer_identity lives
// ONLY on this plugin-level endpoint, not the version-detail one.)
type PluginDetail struct {
	Namespace      string          `json:"namespace"`
	PluginID       string          `json:"plugin_id"`
	LatestVersion  string          `json:"latest_version"`
	SignerIdentity string          `json:"signer_identity"`
	Releases       []PluginRelease `json:"releases"`
}

// PluginRelease is one version in the plugin's release history.
type PluginRelease struct {
	Version     string `json:"version"`
	IndexDigest string `json:"index_digest"`
	Signed      bool   `json:"signed"`
}

// Coordinate returns "<namespace>/<plugin_id>".
func (d *PluginDetail) Coordinate() string { return d.Namespace + "/" + d.PluginID }

// ResolveRelease returns the release to install: the one matching
// requestedVersion, or the latest_version release when requestedVersion is
// empty. Resolving against the authoritative release list (not a client
// guess) is what lets the installer cross-check the hub-recorded
// index_digest against what the registry actually serves.
func (d *PluginDetail) ResolveRelease(requestedVersion string) (*PluginRelease, error) {
	if requestedVersion == "" {
		if d.LatestVersion == "" {
			return nil, fmt.Errorf("plugin %s has no published versions", d.Coordinate())
		}
		for i := range d.Releases {
			if d.Releases[i].Version == d.LatestVersion {
				return &d.Releases[i], nil
			}
		}
		// Hub response normally includes the latest release in the list, but if
		// for some reason the release list doesn't contain it, synthesise a stub so
		// the caller at least has the version string. The empty IndexDigest on this
		// stub disables the install-time registry-divergence cross-check (the caller
		// warns and proceeds); signature + TOFU verification still run.
		return &PluginRelease{Version: d.LatestVersion}, nil
	}
	for i := range d.Releases {
		if d.Releases[i].Version == requestedVersion {
			return &d.Releases[i], nil
		}
	}
	return nil, fmt.Errorf("plugin %s has no version %q (latest is %s)", d.Coordinate(), requestedVersion, d.LatestVersion)
}

// ErrPluginNotFound is returned when the hub has no such plugin coordinate.
var ErrPluginNotFound = fmt.Errorf("plugin not found on grc.store")

// GetPluginDetails fetches GET /v1/plugins/<ns>/<id> from the configured hub
// (anonymous). A 404 yields ErrPluginNotFound (a clear "no such plugin").
func (c *Client) GetPluginDetails(ctx context.Context, namespace, pluginID string) (*PluginDetail, error) {
	var d PluginDetail
	err := c.getJSON(ctx, fmt.Sprintf("/v1/plugins/%s/%s", namespace, pluginID), &d)
	if err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s/%s", ErrPluginNotFound, namespace, pluginID)
		}
		return nil, err
	}
	return &d, nil
}
