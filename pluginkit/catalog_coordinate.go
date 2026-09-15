package pluginkit

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CatalogCoordinate names a control catalog published on grc.store:
// <namespace>/<id>@<version>. The version is the catalog's OCI tag exactly as
// published (e.g. "v2026.08.26"); it is never normalized, because it is also
// the cache filename.
type CatalogCoordinate struct {
	Namespace string
	ID        string
	Version   string
}

// catalogSegmentRegex bounds each coordinate segment to a safe path component,
// since namespace, id and version all become directory or file names in the
// install cache.
var catalogSegmentRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ParseCatalogCoordinate parses "<namespace>/<id>[@<version>]". The version is
// optional here so a policy.catalogs entry can name a bare catalog; AddCatalogs
// requires it.
func ParseCatalogCoordinate(s string) (CatalogCoordinate, error) {
	s = strings.TrimSpace(s)
	repo, version, _ := strings.Cut(s, "@")
	ns, id, ok := strings.Cut(repo, "/")
	if !ok {
		return CatalogCoordinate{}, fmt.Errorf("%q is not a grc.store catalog coordinate; want <namespace>/<id>@<version>", s)
	}
	c := CatalogCoordinate{Namespace: ns, ID: id, Version: version}
	for _, seg := range []string{ns, id} {
		if !catalogSegmentRegex.MatchString(seg) {
			return CatalogCoordinate{}, fmt.Errorf("invalid segment %q in catalog coordinate %q", seg, s)
		}
	}
	if version != "" && !catalogSegmentRegex.MatchString(version) {
		return CatalogCoordinate{}, fmt.Errorf("invalid version %q in catalog coordinate %q", version, s)
	}
	return c, nil
}

// Repository returns "<namespace>/<id>", the catalog's OCI repository path on
// the grc.store registry (catalogs carry no reserved segment; plugins do).
func (c CatalogCoordinate) Repository() string { return c.Namespace + "/" + c.ID }

// String returns the canonical "<namespace>/<id>@<version>" form (no "@" when
// the version is empty).
func (c CatalogCoordinate) String() string {
	if c.Version == "" {
		return c.Repository()
	}
	return c.Repository() + "@" + c.Version
}

// CatalogCachePath is where `pvtr install` writes a verified catalog and where a
// plugin reads it at run time: <binariesDir>/catalogs/<namespace>/<id>/<version>.yaml.
// Both sides go through here so the layout cannot drift.
func CatalogCachePath(binariesDir string, c CatalogCoordinate) string {
	return filepath.Join(binariesDir, "catalogs", c.Namespace, c.ID, c.Version+".yaml")
}
