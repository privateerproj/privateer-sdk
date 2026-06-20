package install

import (
	"fmt"
	"testing"
)

func TestParseCoordinate(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		coord   string
		version string
		wantErr bool
	}{
		{"ns/id latest", "ossf/pvtr-github-repo", "ossf/pvtr-github-repo", "", false},
		{"ns/id pinned", "ossf/pvtr-github-repo@1.4.0", "ossf/pvtr-github-repo", "1.4.0", false},
		{"surrounding space trimmed", "  finos-ccc/ccc-evaluator @ 2.1.0 ", "finos-ccc/ccc-evaluator", "2.1.0", false},
		{"internal space in id rejected", "ns/bad id@1.0", "", "", true},
		{"finos pin", "finos-ccc/ccc-evaluator@2.1.0", "finos-ccc/ccc-evaluator", "2.1.0", false},
		// grc.store has NO default namespace — a bare name is a clear error, not
		// a silently-defaulted "privateerproj/<name>" (the reversed Phase-B break).
		{"bare name rejected", "pvtr-github-repo", "", "", true},
		{"empty", "", "", "", true},
		{"empty namespace", "/id", "", "", true},
		{"empty id", "ns/", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			namespace, pluginId, version, err := parseCoordinate(c.arg)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got namespace=%q, pluginId=%q version=%q", c.arg, namespace, pluginId, version)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", c.arg, err)
			}
			coord := fmt.Sprintf("%s/%s", namespace, pluginId)
			if coord != c.coord || version != c.version {
				t.Errorf("parseCoordinate(%q) = (%q, %q), want (%q, %q)", c.arg, coord, version, c.coord, c.version)
			}
		})
	}
}

func TestParseCoordinate_BareNameMessageIsActionable(t *testing.T) {
	// The bare-name error must point the user at the right form.
	_, _, _, err := parseCoordinate("pvtr-github-repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !contains(got, "<namespace>/<plugin_id>") {
		t.Errorf("bare-name error should show the coordinate form, got: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
