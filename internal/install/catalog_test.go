package install

import (
	"bytes"
	"strings"
	"testing"

	"github.com/revanite-io/grc-store-protocol/pluginspec"
)

func TestCatalogCoordinates_SkipsMalformedWithWarning(t *testing.T) {
	var w bytes.Buffer
	got := catalogCoordinates(&w, []pluginspec.Evaluate{
		{Catalog: "openssf/osps-baseline", CatalogVersion: "v2026.08.26"},
		{Catalog: "openssf/osps-baseline", CatalogVersion: ""},
		{Catalog: "osps-baseline", CatalogVersion: "1"},
	})
	if len(got) != 1 || got[0].String() != "openssf/osps-baseline@v2026.08.26" {
		t.Fatalf("got %+v", got)
	}
	if strings.Count(w.String(), "Warning:") != 2 {
		t.Fatalf("expected two warnings, got:\n%s", w.String())
	}
}
