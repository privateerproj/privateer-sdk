package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/revanite-io/grc-store-protocol/registrytoken"
)

// accessEntry is one Distribution resource-scope grant in the minted token's JWT
// `access` claim. We decode it to learn which actions (pull/push) the hub actually
// granted, since the hub grants pull-only (NOT an error) when the caller doesn't
// own the namespace. This is a local JWT-decoding detail — NOT part of the shared
// grc-store-protocol contract (registrytoken there models only the /v2/token
// response, not the embedded JWT claim).
type accessEntry struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// RegistryToken is a minted zot registry token plus the actions the hub actually
// granted on the plugin repo. GrantsPush reports whether push was granted —
// pvtr checks this BEFORE pushing so an unowned-namespace publish fails fast
// (legibly), instead of minting a pull-only token and failing at the raw
// registry push (after already prompting for a sigstore sign-in).
type RegistryToken struct {
	Token   string
	Actions []string // granted actions on the plugin repo (e.g. ["pull"] or ["pull","push"])
}

// GrantsPush reports whether the minted token authorizes pushing to the plugin
// repo.
func (t RegistryToken) GrantsPush() bool {
	return slices.Contains(t.Actions, "push")
}

// MintRegistryToken exchanges an upstream OIDC bearer for a zot registry token
// scoped to push+pull on the plugin repo, and reports the actions the hub
// actually granted. pvtr does this exchange itself (rather than leaning on oras's
// OAuth2 assumptions) because the hub's /v2/token is a GET realm keyed on the
// Authorization header — minting here and handing oras a ready registry token
// (Credential.AccessToken) is the robust path.
//
// hubURL is the hub base; coordinate is "<ns>/<plugin_id>"; upstreamBearer is
// the device-grant / GHA-OIDC token (empty → an anonymous pull-only token).
// The request is routed through the hub Client's doJSON helper so the transport
// bounds (15s timeout, shared error shape) are consistent with other hub API calls.
func MintRegistryToken(ctx context.Context, hubURL, coordinate, upstreamBearer string) (RegistryToken, error) {
	ns, id, ok := splitCoordinate(coordinate)
	if !ok {
		return RegistryToken{}, fmt.Errorf("invalid coordinate %q for token scope", coordinate)
	}
	repo := pluginRepoPath(ns, id)
	q := url.Values{}
	q.Set("scope", fmt.Sprintf("repository:%s:pull,push", repo))
	q.Set("service", "zot")

	// Build a temporary hub client targeting the caller-supplied URL.
	// doJSON is used for the shared transport bounds and consistent error shape;
	// the bearer is passed as the Authorization header.
	c := newHubClient(hubURL, defaultHubTimeout)
	var tr registrytoken.Response
	if err := c.doJSON(ctx, http.MethodGet, "/v2/token?"+q.Encode(), upstreamBearer, nil, &tr); err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.status == http.StatusUnauthorized {
			return RegistryToken{}, fmt.Errorf("minting registry token: %w — your login may be expired, run `pvtr login`", err)
		}
		return RegistryToken{}, fmt.Errorf("minting registry token: %w", err)
	}
	tok := tr.BearerToken()
	if tok == "" {
		return RegistryToken{}, fmt.Errorf("token response carried no token")
	}
	return RegistryToken{Token: tok, Actions: grantedActions(tok, repo)}, nil
}

// grantedActions decodes the registry token's JWT `access` claim and returns the
// actions granted on repo. The token is a Docker-style JWT we minted for
// ourselves — we read (not verify) the payload to learn our own granted scope.
// Returns nil if the token isn't a decodable JWT or has no entry for repo.
func grantedActions(token, repo string) []string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Access []accessEntry `json:"access"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	for _, e := range claims.Access {
		if e.Type == "repository" && e.Name == repo {
			return e.Actions
		}
	}
	return nil
}
