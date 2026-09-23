package pluginkit

import (
	"path/filepath"
	"testing"
)

func TestParseCatalogCoordinate(t *testing.T) {
	cases := []struct {
		in      string
		want    CatalogCoordinate
		wantErr bool
	}{
		{in: "openssf/osps-baseline@v2026.08.26", want: CatalogCoordinate{"openssf", "osps-baseline", "v2026.08.26"}},
		{in: " openssf/osps-baseline ", want: CatalogCoordinate{"openssf", "osps-baseline", ""}},
		{in: "osps-baseline", wantErr: true},
		{in: "openssf/osps-baseline@../etc", wantErr: true},
		{in: "openssf/a/b@1", wantErr: true},
		{in: "/x@1", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseCatalogCoordinate(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestCatalogCoordinate_StringAndCachePath(t *testing.T) {
	c := CatalogCoordinate{"openssf", "osps-baseline", "v2026.08.26"}
	if c.String() != "openssf/osps-baseline@v2026.08.26" || c.Repository() != "openssf/osps-baseline" {
		t.Fatalf("String=%q Repository=%q", c.String(), c.Repository())
	}
	want := filepath.Join("bin", "catalogs", "openssf", "osps-baseline", "v2026.08.26.yaml")
	if got := CatalogCachePath("bin", c); got != want {
		t.Fatalf("CatalogCachePath = %q, want %q", got, want)
	}
}
