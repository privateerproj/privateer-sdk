package publish

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/verify"
	"github.com/privateerproj/privateer-sdk/pluginkit"
)

const (
	catalogV2 = "metadata:\n  id: osps-baseline\ncontrols:\n  - id: C1\n    assessment-requirements:\n      - id: R-1\n      - id: R-2\n"
	catalogV1 = "metadata:\n  id: osps-baseline\ncontrols:\n  - id: C1\n    assessment-requirements:\n      - id: R-1\n"
)

// stubCatalogs is a catalog.Source serving parsed catalogs keyed by coordinate.
type stubCatalogs map[string]string

func (m stubCatalogs) Fetch(_ context.Context, c pluginkit.CatalogCoordinate) (*verify.VerifiedCatalog, error) {
	y, ok := m[c.String()]
	if !ok {
		return nil, errors.New("not published")
	}
	cat, err := pluginkit.ParseCatalog([]byte(y))
	if err != nil {
		return nil, err
	}
	return &verify.VerifiedCatalog{YAML: []byte(y), Catalog: cat}, nil
}

func TestEvaluatesFromCatalogs_IntersectsStepsPerCatalog(t *testing.T) {
	fetch := stubCatalogs{"openssf/osps-baseline@v2": catalogV2, "openssf/osps-baseline@v1": catalogV1}
	got, err := evaluatesFromCatalogs(context.Background(), fetch, []string{"openssf/osps-baseline@v2", "openssf/osps-baseline@v1"}, []string{"R-2", "R-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Catalog != "openssf/osps-baseline" || got[0].CatalogVersion != "v2" || strings.Join(got[0].RequirementIDs, ",") != "R-1,R-2" {
		t.Errorf("v2 entry = %+v", got[0])
	}
	// R-2 is only in v2; v1 keeps just R-1 and that is not an error.
	if got[1].CatalogVersion != "v1" || strings.Join(got[1].RequirementIDs, ",") != "R-1" {
		t.Errorf("v1 entry = %+v", got[1])
	}
}

func TestEvaluatesFromCatalogs_FailsClosed(t *testing.T) {
	fetch := stubCatalogs{"openssf/osps-baseline@v1": catalogV1}
	cases := map[string]struct {
		coords, steps []string
		wantErr       string
	}{
		"no step in catalog": {[]string{"openssf/osps-baseline@v1"}, []string{"R-9"}, "no evaluation step matches"},
		"unpublished":        {[]string{"openssf/osps-baseline@v7"}, []string{"R-1"}, "not published"},
		"bad coordinate":     {[]string{"osps-baseline"}, []string{"R-1"}, "not a grc.store catalog coordinate"},
	}
	for name, tc := range cases {
		_, err := evaluatesFromCatalogs(context.Background(), fetch, tc.coords, tc.steps)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: got %v, want %q", name, err, tc.wantErr)
		}
	}
}
