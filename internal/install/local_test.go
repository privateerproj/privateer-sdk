package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
)

// stubCatalogSource serves the listed coordinates and reports the rest as not
// published.
type stubCatalogSource map[pluginkit.CatalogCoordinate]bool

func (s stubCatalogSource) Fetch(_ context.Context, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	if !s[c] {
		return nil, oci.ErrCatalogNotFound
	}
	return &verify.VerifiedCatalog{YAML: []byte("metadata:\n  id: " + c.ID + "\n"), SignerIdentity: "keyless:example#wf"}, nil
}

// --local coordinates come only from AddCatalogs, so a catalog the hub does not
// have fails the install, naming every missing one, instead of exiting 0 and
// failing every later `pvtr run`.
func TestInstallDeclaredCatalogs_FailsClosedNamingEveryMissing(t *testing.T) {
	present := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	gone1 := pluginkit.CatalogCoordinate{Namespace: "acme", ID: "gone", Version: "v1"}
	gone2 := pluginkit.CatalogCoordinate{Namespace: "acme", ID: "also-gone", Version: "v2"}
	dir := t.TempDir()

	err := installDeclaredCatalogs(context.Background(), &bytes.Buffer{}, stubCatalogSource{present: true}, dir, []pluginkit.CatalogCoordinate{gone1, present, gone2})
	if !errors.Is(err, oci.ErrCatalogNotFound) {
		t.Fatalf("got %v, want ErrCatalogNotFound", err)
	}
	for _, c := range []pluginkit.CatalogCoordinate{gone1, gone2} {
		if !strings.Contains(err.Error(), c.String()) {
			t.Errorf("error %q does not name %s", err, c)
		}
	}
	if _, err := os.Stat(pluginkit.CatalogCachePath(dir, present)); err != nil {
		t.Errorf("published catalog was not cached: %v", err)
	}
}

func TestInstallDeclaredCatalogs_AllPublished(t *testing.T) {
	c := pluginkit.CatalogCoordinate{Namespace: "openssf", ID: "osps-baseline", Version: "v1"}
	if err := installDeclaredCatalogs(context.Background(), &bytes.Buffer{}, stubCatalogSource{c: true}, t.TempDir(), []pluginkit.CatalogCoordinate{c}); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}
