package oci

import (
	"context"
)

// browsePath is the hub's anonymous plugin-directory endpoint (ADR-0034).
const browsePath = "/v1/plugins"

// BrowseItem is one entry from GET {hub}/v1/plugins. Only the fields pvtr uses
// are decoded. namespace+plugin_id form the install coordinate; this endpoint is
// DISCOVERY, not curation — it lists whatever has been published.
type BrowseItem struct {
	Namespace     string `json:"namespace"`
	PluginID      string `json:"plugin_id"`
	LatestVersion string `json:"latest_version"`
	Signed        bool   `json:"signed"`
}

// Coordinate returns the "<namespace>/<plugin_id>" install coordinate.
func (b BrowseItem) Coordinate() string { return b.Namespace + "/" + b.PluginID }

// browseResponse is the GET /v1/plugins envelope.
type browseResponse struct {
	Items []BrowseItem `json:"items"`
}

// Browse lists the plugins published to the configured hub (anonymous). It is a
// directory for `pvtr list --installable`, NOT an install-time gate — install
// trust comes from the §6 signature/identity verification, never from presence
// in this list.
func (c *Client) Browse(ctx context.Context) ([]BrowseItem, error) {
	var out browseResponse
	if err := c.getJSON(ctx, browsePath, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}
