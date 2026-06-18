package oci

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/revanite-io/grc-store-protocol/syncapi"
)

// Sync tells the hub to ingest a pushed plugin index: it re-fetches the index
// from its own registry, verifies the signature against its embedded trusted
// root, authorizes by namespace ownership, and persists it. upstreamBearer is
// the same OIDC token used for the push (the hub authorizes /sync by namespace
// ownership). hubURL is the hub base (PVTR_HUB_URL).
//
// The hub performs server-side work (signature verification, registry re-fetch)
// so sync gets a 60-second bound — longer than the 15-second default for plain
// hub-API GETs. The bound lives on this call's own http.Client; the request
// still carries the caller's context, so an already-cancelled parent
// short-circuits immediately. The request is routed through the hub Client's
// doJSON helper for a consistent error shape.
func Sync(ctx context.Context, hubURL, coordinate, tag, upstreamBearer string) error {
	ns, id, ok := splitCoordinate(coordinate)
	if !ok {
		return fmt.Errorf("invalid coordinate %q for sync", coordinate)
	}

	// 60-second client timeout (not the shared 15-second NewClient default):
	// http.Client.Timeout fires independently of the context, so reusing the
	// 15-second client would silently cap sync at 15s regardless of any context
	// deadline. The caller's context still propagates via NewRequestWithContext.
	c := newHubClient(hubURL, 60*time.Second)
	path := fmt.Sprintf("/v1/plugins/%s/%s/sync", ns, id)
	body := syncapi.Request{
		Repository: pluginRepoPath(ns, id),
		Tag:        tag,
	}
	if err := c.doJSON(ctx, http.MethodPost, path, upstreamBearer, body, nil); err != nil {
		// The hub returns actionable JSON errors (plugin_unsigned,
		// plugin_signer_mismatch, registry_diverged, …); doJSON captures the
		// response body into the error so they surface verbatim.
		return fmt.Errorf("hub sync of %s:%s: %w", coordinate, tag, err)
	}
	return nil
}
