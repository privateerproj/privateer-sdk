package command

import "testing"

func TestMergeExitCode(t *testing.T) {
	tests := []struct {
		name       string
		prev, next int
		want       int
	}{
		{"TestPass over TestPass keeps TestPass", TestPass, TestPass, TestPass},
		{"TestFail beats TestPass", TestPass, TestFail, TestFail},
		{"TestPass after TestFail keeps TestFail", TestFail, TestPass, TestFail},
		{"BadUsage beats TestFail", TestFail, BadUsage, BadUsage},
		{"TestFail does not downgrade BadUsage", BadUsage, TestFail, BadUsage},
		{"InternalError beats BadUsage", BadUsage, InternalError, InternalError},
		{"BadUsage does not downgrade InternalError", InternalError, BadUsage, InternalError},
		{"InternalError beats TestFail", TestFail, InternalError, InternalError},
		{"InternalError beats TestPass", TestPass, InternalError, InternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeExitCode(tt.prev, tt.next); got != tt.want {
				t.Errorf("mergeExitCode(%d, %d) = %d, want %d", tt.prev, tt.next, got, tt.want)
			}
		})
	}
}

// TestPlanRun covers Run's selection logic (which plugins execute and the
// early-exit codes) directly, without spawning plugin subprocesses or mocking
// the go-plugin RPC chain. The subprocess execution itself stays a thin shell
// in Run around this decision.
func TestPlanRun(t *testing.T) {
	pkg := func(name string, installed, requested bool) *PluginPkg {
		return &PluginPkg{Name: name, Installed: installed, Requested: requested}
	}

	tests := []struct {
		name        string
		plugins     []*PluginPkg
		wantRun     []string // plugin names expected in toRun, in order
		wantExit    int
		wantCulprit string // "" when no culprit expected
	}{
		{
			name:     "empty list returns NoTests",
			plugins:  nil,
			wantRun:  nil,
			wantExit: NoTests,
		},
		{
			name:     "only non-requested plugins run nothing",
			plugins:  []*PluginPkg{pkg("acme/installed-only", true, false)},
			wantRun:  nil,
			wantExit: 0,
		},
		{
			name:     "requested and installed is selected",
			plugins:  []*PluginPkg{pkg("acme/scanner", true, true)},
			wantRun:  []string{"acme/scanner"},
			wantExit: 0,
		},
		{
			name:        "requested but not installed returns BadUsage",
			plugins:     []*PluginPkg{pkg("acme/missing", false, true)},
			wantRun:     nil,
			wantExit:    BadUsage,
			wantCulprit: "acme/missing",
		},
		{
			name: "mixed list selects only requested-and-installed, in order",
			plugins: []*PluginPkg{
				pkg("acme/local-only", true, false),
				pkg("acme/scanner", true, true),
				pkg("acme/second", true, true),
			},
			wantRun:  []string{"acme/scanner", "acme/second"},
			wantExit: 0,
		},
		{
			name: "not-installed requested plugin aborts before running earlier ones",
			plugins: []*PluginPkg{
				pkg("acme/scanner", true, true),
				pkg("acme/missing", false, true),
			},
			wantRun:     nil,
			wantExit:    BadUsage,
			wantCulprit: "acme/missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toRun, earlyExit, culprit := planRun(tt.plugins)

			if earlyExit != tt.wantExit {
				t.Errorf("earlyExit = %d, want %d", earlyExit, tt.wantExit)
			}

			gotNames := make([]string, len(toRun))
			for i, p := range toRun {
				gotNames[i] = p.Name
			}
			if len(gotNames) != len(tt.wantRun) {
				t.Fatalf("toRun = %v, want %v", gotNames, tt.wantRun)
			}
			for i := range gotNames {
				if gotNames[i] != tt.wantRun[i] {
					t.Errorf("toRun[%d] = %q, want %q", i, gotNames[i], tt.wantRun[i])
				}
			}

			gotCulprit := ""
			if culprit != nil {
				gotCulprit = culprit.Name
			}
			if gotCulprit != tt.wantCulprit {
				t.Errorf("culprit = %q, want %q", gotCulprit, tt.wantCulprit)
			}
		})
	}
}
