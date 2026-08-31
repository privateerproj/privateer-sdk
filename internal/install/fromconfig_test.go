package install

import (
	"sort"
	"testing"

	"github.com/spf13/viper"
)

// configureServices sets the viper state missingFromConfig reads: an empty
// binaries dir (so no plugin is installed) and two services requesting
// different plugins, one version-pinned. It resets viper afterwards so tests
// don't leak.
func configureServices(t *testing.T) {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set("binaries-path", t.TempDir())
	viper.Set("services", map[string]interface{}{
		"svc-a": map[string]interface{}{"plugin": "acme/alpha"},
		"svc-b": map[string]interface{}{"plugin": "acme/beta", "version": "1.2.0"},
	})
}

func TestMissingFromConfig_TargetScoping(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   []string
	}{
		{"empty target resolves every service", "", []string{"acme/alpha", "acme/beta@1.2.0"}},
		{"target scopes to the named service", "svc-a", []string{"acme/alpha"}},
		{"target scopes to a version-pinned service", "svc-b", []string{"acme/beta@1.2.0"}},
		{"mixed-case target matches the lowercased service key", "Svc-A", []string{"acme/alpha"}},
		{"unknown target resolves nothing", "ghost", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureServices(t)
			got, err := missingFromConfig(tt.target)
			if err != nil {
				t.Fatalf("missingFromConfig(%q) error = %v", tt.target, err)
			}
			// Service map iteration order is nondeterministic; compare sorted.
			sort.Strings(got)
			if len(got) != len(tt.want) {
				t.Fatalf("missingFromConfig(%q) = %v, want %v", tt.target, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("missingFromConfig(%q)[%d] = %q, want %q", tt.target, i, got[i], tt.want[i])
				}
			}
		})
	}
}
