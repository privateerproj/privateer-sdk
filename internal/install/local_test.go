package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/manifest"
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

func TestLocal_RequireSignedRefuses(t *testing.T) {
	viper.Set("require-signed-plugins", true)
	t.Cleanup(func() { viper.Set("require-signed-plugins", false) })

	err := Local(context.Background(), &bytes.Buffer{}, "/does/not/matter")
	if !errors.Is(err, ErrUnsignedLocalInstall) {
		t.Fatalf("got %v, want ErrUnsignedLocalInstall", err)
	}
}

func TestMoveFileWithCrashProtection_ReturnsDigest(t *testing.T) {
	src := filepath.Join(t.TempDir(), "plugin")
	if err := os.WriteFile(src, []byte("bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest, err := moveFileWithCrashProtection(src, t.TempDir(), "plugin")
	if err != nil {
		t.Fatal(err)
	}
	if want := manifest.DigestBytes([]byte("bytes")); digest != want {
		t.Errorf("got %q, want %q", digest, want)
	}
}
