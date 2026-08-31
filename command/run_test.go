package command

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
)

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
	pkg := func(name, service string, installed, requested bool) *PluginPkg {
		return &PluginPkg{Name: name, ServiceTarget: service, Installed: installed, Requested: requested}
	}

	tests := []struct {
		name       string
		plugins    []*PluginPkg
		target     string
		wantRun    []string // plugin names expected in toRun, in order
		wantExit   int
		wantErrMsg string // "" when no error message expected
	}{
		{
			name:     "empty list returns NoTests",
			plugins:  nil,
			wantRun:  nil,
			wantExit: NoTests,
		},
		{
			name:     "empty list with a target still returns NoTests",
			plugins:  nil,
			target:   "svc-a",
			wantRun:  nil,
			wantExit: NoTests,
		},
		{
			name:     "only non-requested plugins run nothing",
			plugins:  []*PluginPkg{pkg("acme/installed-only", "", true, false)},
			wantRun:  nil,
			wantExit: 0,
		},
		{
			name:     "requested and installed is selected",
			plugins:  []*PluginPkg{pkg("acme/scanner", "svc-a", true, true)},
			wantRun:  []string{"acme/scanner"},
			wantExit: 0,
		},
		{
			name:       "requested but not installed returns BadUsage",
			plugins:    []*PluginPkg{pkg("acme/missing", "svc-a", false, true)},
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: "requested plugin that is not installed: acme/missing (required by targets: svc-a)",
		},
		{
			name: "mixed list selects only requested-and-installed, in order",
			plugins: []*PluginPkg{
				pkg("acme/local-only", "", true, false),
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/second", "svc-b", true, true),
			},
			wantRun:  []string{"acme/scanner", "acme/second"},
			wantExit: 0,
		},
		{
			name: "not-installed requested plugin aborts before running earlier ones",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/missing", "svc-b", false, true),
			},
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: "requested plugin that is not installed: acme/missing (required by targets: svc-b)",
		},
		{
			name:       "service entry with no plugin key reports no plugin configured",
			plugins:    []*PluginPkg{pkg("", "svc-x", false, true)},
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: "requested plugin that is not installed: no plugin configured (required by targets: svc-x)",
		},
		{
			name: "all missing plugins are reported with their targets, sorted",
			plugins: []*PluginPkg{
				pkg("acme/gone", "b-svc", false, true),
				pkg("acme/gone", "a-svc", false, true),
				pkg("acme/absent", "z-svc", false, true),
			},
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: "requested plugins that are not installed: acme/absent (required by targets: z-svc), acme/gone (required by targets: a-svc, b-svc)",
		},
		{
			name: "target scopes the run to the named service",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/second", "svc-b", true, true),
			},
			target:   "svc-a",
			wantRun:  []string{"acme/scanner"},
			wantExit: 0,
		},
		{
			name: "target ignores another service's missing plugin",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/missing", "svc-b", false, true),
			},
			target:   "svc-a",
			wantRun:  []string{"acme/scanner"},
			wantExit: 0,
		},
		{
			name: "target whose own plugin is missing returns BadUsage",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/missing", "svc-b", false, true),
			},
			target:     "svc-b",
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: "requested plugin that is not installed: acme/missing (required by targets: svc-b)",
		},
		{
			name: "mixed-case target matches the viper-lowercased service key",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/second", "svc-b", true, true),
			},
			target:   "Svc-A",
			wantRun:  []string{"acme/scanner"},
			wantExit: 0,
		},
		{
			name: "unknown target returns BadUsage listing available targets",
			plugins: []*PluginPkg{
				pkg("acme/scanner", "svc-a", true, true),
				pkg("acme/second", "svc-b", true, true),
			},
			target:     "ghost",
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: `target "ghost" is not defined in the config (available targets: svc-a, svc-b)`,
		},
		{
			name:       "unknown target with no requested plugins omits the available list",
			plugins:    []*PluginPkg{pkg("acme/installed-only", "", true, false)},
			target:     "ghost",
			wantRun:    nil,
			wantExit:   BadUsage,
			wantErrMsg: `target "ghost" is not defined in the config`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toRun, earlyExit, errMsg := planRun(tt.plugins, tt.target)

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

			if errMsg != tt.wantErrMsg {
				t.Errorf("errMsg = %q, want %q", errMsg, tt.wantErrMsg)
			}
		})
	}
}

// TestRun_ConsultsTargetName proves Run wires config.TargetName() into the plan:
// a target set via the config/env tier (viper) that names no configured service
// must abort with BadUsage before any plugin is considered runnable.
func TestRun_ConsultsTargetName(t *testing.T) {
	resetViper()
	t.Cleanup(resetViper)
	viper.Set("target", "ghost")

	getPlugins := func() []*PluginPkg {
		return []*PluginPkg{{Name: "acme/scanner", ServiceTarget: "svc-a", Installed: true, Requested: true}}
	}
	if code := Run(hclog.NewNullLogger(), getPlugins); code != BadUsage {
		t.Errorf("Run with an unknown target = %d, want BadUsage (%d)", code, BadUsage)
	}
}

// pkgs builds n minimal PluginPkgs; the run funcs under test never touch their
// fields beyond identity, so empty structs suffice.
func pkgs(n int) []*PluginPkg {
	out := make([]*PluginPkg, n)
	for i := range out {
		out[i] = &PluginPkg{}
	}
	return out
}

func TestRunSequential(t *testing.T) {
	t.Run("merges exit codes in order", func(t *testing.T) {
		codes := []int{TestPass, TestFail, TestPass}
		var ran []int
		got := runSequential(pkgs(3), func(num int, _ *PluginPkg) (int, bool) {
			ran = append(ran, num)
			return codes[num-1], false
		})
		if got != TestFail {
			t.Errorf("exit = %d, want %d", got, TestFail)
		}
		if len(ran) != 3 || ran[0] != 1 || ran[1] != 2 || ran[2] != 3 {
			t.Errorf("ran = %v, want [1 2 3]", ran)
		}
	})

	t.Run("plugin internal error merges and the run continues", func(t *testing.T) {
		var calls int
		got := runSequential(pkgs(3), func(num int, _ *PluginPkg) (int, bool) {
			calls++
			if num == 2 {
				return InternalError, false
			}
			return TestPass, false
		})
		if got != InternalError {
			t.Errorf("exit = %d, want %d", got, InternalError)
		}
		if calls != 3 {
			t.Errorf("calls = %d, want 3 (a plugin's own InternalError must not abort)", calls)
		}
	})

	t.Run("host-level failure aborts", func(t *testing.T) {
		var calls int
		got := runSequential(pkgs(3), func(num int, _ *PluginPkg) (int, bool) {
			calls++
			if num == 2 {
				return InternalError, true
			}
			return TestPass, false
		})
		if got != InternalError {
			t.Errorf("exit = %d, want %d", got, InternalError)
		}
		if calls != 2 {
			t.Errorf("calls = %d, want 2 (abort after the failing plugin)", calls)
		}
	})
}

// TestConcurrencyLimit covers the default Run relies on: an unset key must read
// as 1 (sequential), not viper's zero value — and invalid values must fail
// closed into an error (Run returns BadUsage), never fall through to 0 and
// silently switch on parallel execution. Valid values pass through — Run routes
// on them with a single `!= 1`, and both arms are covered by TestRunSequential
// and TestRunParallel.
func TestConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name    string
		set     any // nil leaves the key unset
		want    int
		wantErr bool
	}{
		{"unset key defaults to 1", nil, 1, false},
		{"explicit 1", 1, 1, false},
		{"0 passes through", 0, 0, false},
		{"above 1 passes through", 4, 4, false},
		{"non-integer fails closed", "banana", 0, true},
		{"negative fails closed", -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()
			t.Cleanup(resetViper)
			if tt.set != nil {
				viper.Set("concurrency", tt.set)
			}
			got, err := concurrencyLimit()
			if (err != nil) != tt.wantErr {
				t.Fatalf("concurrencyLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("concurrencyLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunParallel(t *testing.T) {
	t.Run("bounds concurrency and merges all results", func(t *testing.T) {
		var inFlight, peak, calls atomic.Int32
		got := runParallel(pkgs(6), 2, func(num int, _ *PluginPkg) (int, bool) {
			calls.Add(1)
			n := inFlight.Add(1)
			for p := peak.Load(); n > p; p = peak.Load() {
				if peak.CompareAndSwap(p, n) {
					break
				}
			}
			defer inFlight.Add(-1)
			if num == 3 {
				// fatal=true: even a host-level failure must not abort parallel mode.
				return InternalError, true
			}
			return TestPass, false
		})
		if got != InternalError {
			t.Errorf("exit = %d, want %d (no early abort in parallel mode)", got, InternalError)
		}
		if calls.Load() != 6 {
			t.Errorf("calls = %d, want 6", calls.Load())
		}
		if peak.Load() > 2 {
			t.Errorf("peak in-flight = %d, want <= 2", peak.Load())
		}
	})

	t.Run("limit 0 runs one per CPU", func(t *testing.T) {
		n := runtime.NumCPU()
		var barrier sync.WaitGroup
		barrier.Add(n)
		got := runParallel(pkgs(n), 0, func(int, *PluginPkg) (int, bool) {
			// Deadlocks (and fails the test by timeout) unless all NumCPU run
			// simultaneously.
			barrier.Done()
			barrier.Wait()
			return TestPass, false
		})
		if got != TestPass {
			t.Errorf("exit = %d, want %d", got, TestPass)
		}
	})

	// The safety cap applies to an explicit limit: maxConcurrency, or NumCPU on
	// hosts larger than that. Every worker blocks until cap are in flight, so
	// an uncapped pool would admit all n and show a peak above the ceiling
	// rather than deadlocking. Sizing the barrier off the effective cap (not a
	// constant) keeps this from hanging on >100-core hosts.
	t.Run("caps an explicit limit at max(maxConcurrency, NumCPU)", func(t *testing.T) {
		cap := max(maxConcurrency, runtime.NumCPU())
		n := cap + 10
		var inFlight, peak atomic.Int32
		var once sync.Once
		release := make(chan struct{})
		runParallel(pkgs(n), n, func(int, *PluginPkg) (int, bool) {
			cur := inFlight.Add(1)
			for p := peak.Load(); cur > p; p = peak.Load() {
				if peak.CompareAndSwap(p, cur) {
					break
				}
			}
			if cur >= int32(cap) {
				once.Do(func() { close(release) })
			}
			<-release
			inFlight.Add(-1)
			return TestPass, false
		})
		if int(peak.Load()) != cap {
			t.Errorf("peak in-flight = %d, want exactly %d", peak.Load(), cap)
		}
	})
}
