package oci

import (
	"context"
	"errors"
	"fmt"
)

// CatalogDetail is the hub's catalog-level record (GET /v1/catalogs/<ns>/<id>):
// the published versions with their manifest digests, and the signer identity
// the hub verified at ingest. Only the fields the catalog installer uses are
// decoded.
type CatalogDetail struct {
	Namespace      string           `json:"namespace"`
	CatalogID      string           `json:"catalog_id"`
	LatestVersion  string           `json:"latest_version"`
	SignerIdentity string           `json:"signer_identity"`
	Releases       []CatalogRelease `json:"releases"`
}

// CatalogRelease is one published version of a catalog.
type CatalogRelease struct {
	Version        string `json:"version"`
	ManifestDigest string `json:"manifest_digest"`
}

// ErrCatalogNotFound is returned when the hub has no such catalog, or no such
// version of it.
var ErrCatalogNotFound = errors.New("catalog not found on grc.store")

// Release returns the release tagged version. The tag is matched verbatim: it
// is the OCI tag to pull. There is no "latest" fallback — a catalog is always
// fetched at a pinned version, and Fetch rejects an empty one before it gets
// here, so resolving a blank version would only produce a cache entry at a path
// nothing reads.
func (d *CatalogDetail) Release(version string) (*CatalogRelease, error) {
	for i := range d.Releases {
		if d.Releases[i].Version == version {
			return &d.Releases[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s/%s has no version %q (latest is %s)", ErrCatalogNotFound, d.Namespace, d.CatalogID, version, d.LatestVersion)
}

// GetCatalogDetails fetches GET /v1/catalogs/<ns>/<id> from the configured hub
// (anonymous). A 404 yields ErrCatalogNotFound.
func (c *Client) GetCatalogDetails(ctx context.Context, namespace, catalogID string) (*CatalogDetail, error) {
	var d CatalogDetail
	notFound := fmt.Errorf("%w: %s/%s", ErrCatalogNotFound, namespace, catalogID)
	if err := c.getJSONOr404(ctx, fmt.Sprintf("/v1/catalogs/%s/%s", namespace, catalogID), &d, notFound); err != nil {
		return nil, err
	}
	return &d, nil
}
