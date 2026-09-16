package publish

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

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
	return install.RunPublishManifest(ctx, host.Path)
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

// uiBase returns the hub's web-UI base: the advertised ui_url when the
// discovery doc carries one (the protocol tells clients to prefer it), else
// derived from the hub's self-reported URL by dropping a leading "hub." label
// (grc.store convention: hub.<env>.grc.store → <env>.grc.store). Best-effort —
// it only points a user at where to claim a namespace.
func uiBase(advertised, hubURL string) string {
	if u := strings.TrimRight(strings.TrimSpace(advertised), "/"); u != "" {
		return u
	}
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

// evaluatesFromCatalogs builds one evaluates entry per declared catalog
// coordinate: the catalog's assessment-requirement ids intersected with the
// plugin's step keys, sorted. A catalog no step matches is an error, and so is
// a step key that matches no catalog at all — both mean the plugin's steps and
// its catalogs disagree, which must be fixed in code rather than published.
//
// preMatched are requirement ids already linked by the deprecated embedded
// path. They count as covered for the orphan check but produce no entry here,
// since the embedded path emitted theirs already.
func evaluatesFromCatalogs(ctx context.Context, fetch func(context.Context, pluginkit.CatalogCoordinate) ([]byte, error), coordinates, steps, preMatched []string) ([]pluginspec.Evaluate, error) {
	matched := make(map[string]bool, len(preMatched))
	for _, id := range preMatched {
		matched[id] = true
	}
	out := make([]pluginspec.Evaluate, 0, len(coordinates))
	for _, raw := range coordinates {
		c, err := pluginkit.ParseCatalogCoordinate(raw)
		if err != nil {
			return nil, err
		}
		data, err := fetch(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("reading declared catalog %s: %w", c, err)
		}
		cat, err := pluginkit.ParseCatalog(data)
		if err != nil {
			return nil, fmt.Errorf("parsing declared catalog %s: %w", c, err)
		}
		inCatalog := map[string]bool{}
		for _, ctrl := range cat.Controls {
			for _, req := range ctrl.AssessmentRequirements {
				inCatalog[req.Id] = true
			}
		}
		var reqs []string
		for _, s := range steps {
			if inCatalog[s] {
				reqs = append(reqs, s)
				matched[s] = true
			}
		}
		if len(reqs) == 0 {
			return nil, fmt.Errorf("no evaluation step matches any assessment requirement in declared catalog %s", c)
		}
		slices.Sort(reqs)
		out = append(out, pluginspec.Evaluate{Catalog: c.Repository(), CatalogVersion: c.Version, RequirementIDs: reqs})
	}
	var orphans []string
	for _, s := range steps {
		if !matched[s] {
			orphans = append(orphans, s)
		}
	}
	if len(orphans) > 0 {
		return nil, fmt.Errorf("evaluation steps %v match no assessment requirement in any catalog the plugin declares or embeds", orphans)
	}
	return out, nil
}
