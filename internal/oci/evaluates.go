package oci

import (
	"fmt"
	"strings"
)

// ValidateForPublish enforces the hub's ingest contract BEFORE assembly/push/
// sign, so a malformed index never reaches the registry (avoiding orphaned,
// signed-but-rejected bytes). It mirrors the checks the hub runs at /sync:
// non-empty evaluates with namespaced catalogs + at least one requirement id
// each, a present entrypoint, and version == the resolved tag. Pure (no
// network); called by `pvtr publish` first.
func ValidateForPublish(p AssembleParams) error {
	if strings.TrimSpace(p.Coordinate) == "" {
		return fmt.Errorf("coordinate is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("version is required")
	}
	// grc.store requires a license on every publication (the hub returns
	// apierror.LicenseRequired at /sync without one). Presence is mirrored here;
	// the SPDX-expression format is validated upstream by pvtr publish.
	if strings.TrimSpace(p.License) == "" {
		return fmt.Errorf("a license is required: set orchestrator.License to an SPDX expression (e.g. \"Apache-2.0\")")
	}
	if len(p.Binaries) == 0 {
		return fmt.Errorf("no binaries to publish (is the GoReleaser dist empty?)")
	}
	for _, b := range p.Binaries {
		if strings.TrimSpace(b.Entrypoint) == "" {
			return fmt.Errorf("binary for %s/%s has no entrypoint name", b.OS, b.Arch)
		}
	}
	// The load-bearing gap that caused the 422: a plugin MUST declare what it
	// evaluates, with namespaced catalogs and requirement ids. This data comes
	// from the plugin itself (embedded reference catalogs, namespaced under
	// orchestrator.Publisher), so an empty list means the plugin loaded no
	// reference catalogs.
	if len(p.Evaluates) == 0 {
		return fmt.Errorf("a plugin must declare what it evaluates (the hub rejects an empty evaluates list): load reference catalogs with AddReferenceCatalogs and set orchestrator.Publisher")
	}
	for i, e := range p.Evaluates {
		if !strings.Contains(strings.TrimSuffix(e.Catalog, "/"), "/") || strings.HasPrefix(e.Catalog, "/") {
			return fmt.Errorf("evaluates[%d].catalog %q must be <namespace>/<catalog_id>", i, e.Catalog)
		}
		if strings.TrimSpace(e.CatalogVersion) == "" {
			return fmt.Errorf("evaluates[%d] (%s) has no catalog_version", i, e.Catalog)
		}
		if len(e.RequirementIDs) == 0 {
			return fmt.Errorf("evaluates[%d] (%s) lists no requirement_ids", i, e.Catalog)
		}
	}
	return nil
}
