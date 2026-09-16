package publish

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/pluginkit"
)

const (
	catalogV2 = "metadata:\n  id: osps-baseline\ncontrols:\n  - id: C1\n    assessment-requirements:\n      - id: R-1\n      - id: R-2\n"
	catalogV1 = "metadata:\n  id: osps-baseline\ncontrols:\n  - id: C1\n    assessment-requirements:\n      - id: R-1\n"
)

func stubCatalogs(m map[string]string) func(context.Context, pluginkit.CatalogCoordinate) ([]byte, error) {
	return func(_ context.Context, c pluginkit.CatalogCoordinate) ([]byte, error) {
		if y, ok := m[c.String()]; ok {
			return []byte(y), nil
		}
		return nil, errors.New("not published")
	}
}

func TestEvaluatesFromCatalogs_IntersectsStepsPerCatalog(t *testing.T) {
	fetch := stubCatalogs(map[string]string{"openssf/osps-baseline@v2": catalogV2, "openssf/osps-baseline@v1": catalogV1})
	got, err := evaluatesFromCatalogs(context.Background(), fetch, []string{"openssf/osps-baseline@v2", "openssf/osps-baseline@v1"}, []string{"R-2", "R-1"}, nil)
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
	fetch := stubCatalogs(map[string]string{"openssf/osps-baseline@v1": catalogV1})
	cases := map[string]struct {
		coords, steps []string
		wantErr       string
	}{
		"orphan step":        {[]string{"openssf/osps-baseline@v1"}, []string{"R-1", "R-9"}, "[R-9] match no assessment requirement"},
		"no step in catalog": {[]string{"openssf/osps-baseline@v1"}, []string{"R-9"}, "no evaluation step matches"},
		"unpublished":        {[]string{"openssf/osps-baseline@v7"}, []string{"R-1"}, "not published"},
		"bad coordinate":     {[]string{"osps-baseline"}, []string{"R-1"}, "not a grc.store catalog coordinate"},
	}
	for name, tc := range cases {
		_, err := evaluatesFromCatalogs(context.Background(), fetch, tc.coords, tc.steps, nil)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: got %v, want %q", name, err, tc.wantErr)
		}
	}
}

// A plugin migrating one catalog at a time has steps that only its embedded
// catalog covers. Those arrive as preMatched and must not be reported as
// orphans, or a mixed plugin cannot publish at all.
func TestEvaluatesFromCatalogs_EmbeddedStepsAreNotOrphans(t *testing.T) {
	fetch := stubCatalogs(map[string]string{"openssf/osps-baseline@v1": catalogV1})
	steps := []string{"R-1", "EMB-1"}

	if _, err := evaluatesFromCatalogs(context.Background(), fetch, []string{"openssf/osps-baseline@v1"}, steps, nil); err == nil {
		t.Fatal("without the embedded linkage EMB-1 must still be an orphan")
	}

	got, err := evaluatesFromCatalogs(context.Background(), fetch, []string{"openssf/osps-baseline@v1"}, steps, []string{"EMB-1"})
	if err != nil {
		t.Fatalf("EMB-1 is linked by the embedded catalog, so this must publish: %v", err)
	}
	// The embedded requirement is already in its own evaluates entry; it must not
	// be duplicated into the declared catalog's.
	if len(got) != 1 || strings.Join(got[0].RequirementIDs, ",") != "R-1" {
		t.Errorf("declared entry = %+v", got)
	}
}
