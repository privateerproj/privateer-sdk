package pluginkit

import (
	"fmt"
	"slices"
	"strings"
)

// PublishManifestCommand is the plugin subcommand that emits the grc.store
// publish manifest as JSON. command.NewPluginCommands wires it onto every
// plugin, and `pvtr publish` execs it on the built binary to read the plugin's
// coordinate and evaluated catalogs — rather than taking them as flags a
// non-owner could forge. Shared here so the producer (publish) and the plugin
// command that emits it can't drift on the name.
const PublishManifestCommand = "publish-manifest"

// PublishManifest is the machine-readable descriptor `pvtr publish` reads from a
// built plugin (via the publish-manifest subcommand) in place of CLI flags. The
// plugin coordinate is Publisher + PluginName. Each evaluated catalog's
// coordinate is the catalog's OWN owner — metadata.author.id — plus its id, so a
// plugin that evaluates someone else's catalog links to the real owner instead
// of falsely claiming it under the plugin's own namespace. A CatalogNamespaces
// override is available for the rare case where a catalog's author.id does not
// match the namespace it is published under on grc.store.
type PublishManifest struct {
	// Coordinate is the plugin's grc.store coordinate "<publisher>/<plugin_id>".
	Coordinate string `json:"coordinate"`
	// License is the plugin's declared publication license, the raw SPDX
	// expression from orchestrator.License. It is carried verbatim here; pvtr
	// validates and canonicalizes it at publish time (grc-store-protocol/spdx is
	// not imported into the plugin binary).
	License string `json:"license"`
	// Evaluates is the control-catalog linkage, deterministically ordered.
	Evaluates []EvaluatesDeclaration `json:"evaluates"`
}

// EvaluatesDeclaration is one control-catalog linkage in the publish manifest.
// The JSON shape matches the OCI config blob's evaluates entry so the publish
// command can map it across without translation noise.
type EvaluatesDeclaration struct {
	Catalog        string   `json:"catalog"`         // "<namespace>/<catalog_id>"
	CatalogVersion string   `json:"catalog_version"` //nolint:tagliatelle // wire contract
	RequirementIDs []string `json:"requirement_ids"` //nolint:tagliatelle // wire contract
}

// PublishManifest assembles the publish descriptor from the orchestrator's
// Publisher + PluginName and its embedded reference catalogs: the coordinate is
// <Publisher>/<PluginName>, and each evaluated catalog is namespaced under its
// own owner (metadata.author.id), with id/version/control-ids read from the
// catalog itself. It fails closed (no manifest) when Publisher or PluginName is
// unset, when no catalogs are loaded, when a catalog has no version or controls,
// or when a catalog has no author id and no CatalogNamespaces override (we will
// not synthesize an owner — claiming an unattributed catalog under any namespace
// is a false claim). Every case is a "this plugin cannot be published yet" error
// the author fixes in code or catalog metadata. A plugin RUNS without these; it
// cannot be PUBLISHED without them.
func (v *EvaluationOrchestrator) PublishManifest() (PublishManifest, error) {
	publisher := strings.TrimSpace(v.Publisher)
	if publisher == "" {
		return PublishManifest{}, fmt.Errorf("plugin declares no grc.store Publisher (author/org id); set orchestrator.Publisher before it can be published")
	}
	pluginID := strings.TrimSpace(v.PluginName)
	if pluginID == "" {
		return PublishManifest{}, fmt.Errorf("plugin has no PluginName to use as its grc.store plugin id")
	}
	if strings.Contains(publisher, "/") || strings.Contains(pluginID, "/") {
		return PublishManifest{}, fmt.Errorf("publisher %q and pluginName %q must not contain '/': the coordinate is exactly <publisher>/<plugin_id>", publisher, pluginID)
	}
	license := strings.TrimSpace(v.License)
	if license == "" {
		return PublishManifest{}, fmt.Errorf("plugin declares no License (grc.store requires one on every publication); set orchestrator.License to an SPDX expression (e.g. \"Apache-2.0\") before it can be published")
	}
	if len(v.referenceCatalogs) == 0 {
		return PublishManifest{}, fmt.Errorf("plugin has no reference catalogs, so it evaluates nothing and cannot be published; load catalogs with AddReferenceCatalogs first")
	}

	evals := make([]EvaluatesDeclaration, 0, len(v.referenceCatalogs))
	for id, catalog := range v.referenceCatalogs {
		version := strings.TrimSpace(catalog.Metadata.Version)
		if version == "" {
			return PublishManifest{}, fmt.Errorf("evaluated catalog %q has no metadata.version", id)
		}

		// Requirement ids are the catalog's OWN control ids. Deduplicated as cheap
		// hardening against any future source of duplicate ids. addEvaluationSuite
		// no longer mutates the shared catalog (copy-on-import), so referenceCatalogs
		// entries always contain only the catalog's own controls here.
		seen := map[string]bool{}
		reqs := make([]string, 0, len(catalog.Controls))
		for _, c := range catalog.Controls {
			if c.Id != "" && !seen[c.Id] {
				seen[c.Id] = true
				reqs = append(reqs, c.Id)
			}
		}
		if len(reqs) == 0 {
			return PublishManifest{}, fmt.Errorf("evaluated catalog %q declares no controls", id)
		}
		slices.Sort(reqs)

		// Resolve the catalog's owning namespace for an ACCURATE cross-link. The
		// catalog itself is the source of truth via metadata.author.id; an explicit
		// CatalogNamespaces override wins for the rare case where author.id doesn't
		// match the namespace the catalog is published under on grc.store. We never
		// fall back to the plugin's own Publisher — claiming a catalog we don't own
		// under our namespace is a false attribution. Fail closed if neither yields
		// a namespace, rather than synthesizing one.
		ns := strings.TrimSpace(catalog.Metadata.Author.Id)
		if override, ok := v.CatalogNamespaces[id]; ok {
			override = strings.TrimSpace(override)
			if override == "" || strings.Contains(override, "/") {
				return PublishManifest{}, fmt.Errorf("CatalogNamespaces[%q] = %q is not a valid namespace", id, override)
			}
			ns = override
		}
		if ns == "" {
			return PublishManifest{}, fmt.Errorf("evaluated catalog %q has no metadata.author.id and no CatalogNamespaces override; cannot determine its owning grc.store namespace (set the catalog's author id, or map it in CatalogNamespaces)", id)
		}
		if strings.Contains(ns, "/") {
			return PublishManifest{}, fmt.Errorf("catalog %q owner namespace %q (from metadata.author.id) must not contain '/'", id, ns)
		}

		evals = append(evals, EvaluatesDeclaration{
			Catalog:        ns + "/" + id,
			CatalogVersion: version,
			RequirementIDs: reqs,
		})
	}
	// Stable order: referenceCatalogs is a map, so sort the output so the manifest
	// (and the downstream signed config blob) is byte-deterministic.
	slices.SortFunc(evals, func(a, b EvaluatesDeclaration) int { return strings.Compare(a.Catalog, b.Catalog) })

	return PublishManifest{Coordinate: publisher + "/" + pluginID, License: license, Evaluates: evals}, nil
}
