package pluginkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/shared"
)

// cachedCatalog writes one catalog into the install cache under binariesDir
// the way `pvtr install` does, with a single requirement the test steps cover.
func cachedCatalog(t *testing.T, binariesDir string, c CatalogCoordinate) {
	t.Helper()
	path := CatalogCachePath(binariesDir, c)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "metadata:\n  id: " + c.ID + "\n  version: " + c.Version + "\ncontrols:\n" +
		"  - id: CCC.Core.C01\n    title: T\n    objective: O\n    assessment-requirements:\n" +
		"      - id: CCC.Core.C01.TR01\n        text: t\n        applicability: [tlp-green]\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalogTestConfig(t *testing.T, binariesDir string, catalogs []string) {
	t.Helper()
	setBasicConfig()
	viper.Set("binaries-path", binariesDir)
	viper.Set("policy.catalogs", catalogs)
	t.Cleanup(func() {
		viper.Set("binaries-path", "")
		viper.Set("policy.catalogs", requestedCatalogs)
	})
}

func TestAddCatalogs_Validation(t *testing.T) {
	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddCatalogs("openssf/osps-baseline"); err == nil || !strings.Contains(err.Error(), "no version") {
		t.Errorf("version required: %v", err)
	}
	if err := orch.AddCatalogs("openssf/osps-baseline@v1", "openssf/osps-baseline@v1"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate: %v", err)
	}
	if err := orch.AddEvaluationSuite("openssf/osps-baseline@v9", nil, nil); err == nil {
		t.Error("a suite for an undeclared coordinate must fail")
	}
}

func TestAddCatalogs_RejectedBatchRegistersNothing(t *testing.T) {
	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddCatalogs("openssf/good@v1", "openssf/bad"); err == nil {
		t.Fatal("expected an error for the versionless second coordinate")
	}
	if len(orch.catalogCoordinates) != 0 {
		t.Errorf("a rejected batch must leave nothing declared, got %v", orch.catalogCoordinates)
	}
	// The duplicate check still covers duplicates within one call, not just
	// against what was declared before it.
	if err := orch.AddCatalogs("openssf/a@v1", "openssf/a@v1"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate within one call: %v", err)
	}
	if len(orch.catalogCoordinates) != 0 {
		t.Errorf("a rejected batch must leave nothing declared, got %v", orch.catalogCoordinates)
	}
}

func TestAddCatalogs_MobilizeLoadsFromCacheAndMatchesConfig(t *testing.T) {
	dir := t.TempDir()
	v2 := CatalogCoordinate{"openssf", "osps-baseline", "v2"}
	v1 := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, v2)
	cachedCatalog(t, dir, v1)
	// Two declared versions make the bare coordinate ambiguous, so the policy
	// names the version.
	catalogTestConfig(t, dir, []string{v2.String()})

	orch := &EvaluationOrchestrator{PluginName: "p", Publisher: "acme", License: "Apache-2.0"}
	if err := orch.AddCatalogs(v1.String(), v2.String()); err != nil {
		t.Fatal(err)
	}
	steps := map[string][]gemara.AssessmentStep{"CCC.Core.C01.TR01": {step_Pass}}
	if err := orch.AddEvaluationSuiteForAllCatalogs(nil, steps); err != nil {
		t.Fatal(err)
	}
	if err := orch.AddEvaluationSuite(v2.String(), nil, steps); err != nil {
		t.Fatal(err)
	}
	if len(orch.pendingSuites) != 2 {
		t.Fatalf("pending suites = %d, want 2 (deduplicated)", len(orch.pendingSuites))
	}

	// Nothing is loaded before Mobilize, yet the manifest is complete.
	m, err := orch.PublishManifest()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.Catalogs, ",") != "openssf/osps-baseline@v1,openssf/osps-baseline@v2" || strings.Join(m.Steps, ",") != "CCC.Core.C01.TR01" || len(m.Evaluates) != 0 {
		t.Errorf("manifest = %+v", m)
	}

	if err := orch.Mobilize(); err != nil {
		t.Fatalf("Mobilize: %v", err)
	}
	if len(orch.Evaluation_Suites) != 1 || orch.Evaluation_Suites[0].CatalogId != v2.String() {
		t.Fatalf("ran suites %+v, want just %s", orch.Evaluation_Suites, v2)
	}
	log := orch.Evaluation_Suites[0].EvaluationLog
	if log.Evaluations[0].Control.ReferenceId != v2.String() {
		t.Errorf("control reference id = %q, want the coordinate", log.Evaluations[0].Control.ReferenceId)
	}
	for _, alias := range []string{"osps-baseline", "openssf/osps-baseline"} {
		if got := orch.matchSuites(alias); len(got) != 2 {
			t.Errorf("%q should match both declared versions (ambiguous), got %v", alias, got)
		}
	}
	if got := orch.matchSuites(v1.String()); len(got) != 1 || got[0].CatalogId != v1.String() {
		t.Errorf("exact coordinate should resolve to v1, got %v", got)
	}
	if got := orch.matchSuites("nope"); len(got) != 0 {
		t.Errorf("unknown catalog must not match, got %v", got)
	}
}

func TestAddCatalogs_MissingCacheNamesTheFix(t *testing.T) {
	catalogTestConfig(t, t.TempDir(), []string{"openssf/other@v1"})
	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddCatalogs("openssf/other@v1"); err != nil {
		t.Fatal(err)
	}
	if err := orch.AddEvaluationSuite("openssf/other@v1", nil, map[string][]gemara.AssessmentStep{"x": {step_Pass}}); err != nil {
		t.Fatal(err)
	}
	err := orch.Mobilize()
	if err == nil || !strings.Contains(err.Error(), "not installed") || !strings.Contains(err.Error(), "pvtr install") {
		t.Fatalf("expected a not-installed error, got %v", err)
	}
}

// A signed catalog is not necessarily a usable one. Mobilize applies the same
// checks to a cached catalog that AddEvaluationSuite applies to an embedded one.
func TestMobilize_RejectsEmptyCachedCatalog(t *testing.T) {
	for name, yaml := range map[string]string{
		"no controls": "metadata:\n  id: osps-baseline\ncontrols: []\n",
		"no id":       "metadata:\n  version: v1\ncontrols:\n  - id: C01\n    title: T\n    objective: O\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			c := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
			path := CatalogCachePath(dir, c)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			catalogTestConfig(t, dir, []string{c.String()})
			orch := &EvaluationOrchestrator{PluginName: "p"}
			if err := orch.AddCatalogs(c.String()); err != nil {
				t.Fatal(err)
			}
			err := orch.Mobilize()
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected a BAD_CATALOG error mentioning %q, got %v", name, err)
			}
			if got := ExitCodeFor(orch, err); got != shared.BadUsage {
				t.Errorf("exit code = %d, want BadUsage (%d)", got, shared.BadUsage)
			}
		})
	}
}

// An embedded catalog may import controls from a catalog the plugin declares
// with AddCatalogs. Imports name a catalog by metadata id and the declared one
// is keyed by coordinate, and it is not loaded until Mobilize, so both the
// lookup and its timing matter. The shared reference entry stays pristine.
func TestMobilize_DeclaredCatalogSatisfiesImports(t *testing.T) {
	dir := t.TempDir()
	dep := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, dep)
	catalogTestConfig(t, dir, []string{"primary"})

	primary, err := ParseCatalog([]byte("metadata:\n  id: primary\n  version: v1\ncontrols:\n" +
		"  - id: P01\n    title: T\n    objective: O\n    assessment-requirements:\n" +
		"      - id: P01.TR01\n        text: t\n        applicability: [tlp-green]\n" +
		"imports:\n  - reference-id: osps-baseline\n    entries:\n      - reference-id: CCC.Core.C01\n"))
	if err != nil {
		t.Fatal(err)
	}
	orch := &EvaluationOrchestrator{PluginName: "p", referenceCatalogs: map[string]*gemara.ControlCatalog{"primary": primary}}
	if err := orch.AddCatalogs(dep.String()); err != nil {
		t.Fatal(err)
	}
	steps := map[string][]gemara.AssessmentStep{"P01.TR01": {step_Pass}, "CCC.Core.C01.TR01": {step_Pass}}
	if err := orch.AddEvaluationSuite("primary", nil, steps); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // twice: resolution must not stack imports
		if err := orch.Mobilize(); err != nil {
			t.Fatalf("Mobilize: %v", err)
		}
		if got := len(orch.Evaluation_Suites[0].EvaluationLog.Evaluations); got != 2 {
			t.Fatalf("run %d: evaluated %d controls, want 2 (own + imported)", i, got)
		}
	}
	if len(primary.Controls) != 1 {
		t.Errorf("reference catalog was mutated: %d controls", len(primary.Controls))
	}
}

// Two policy entries that alias one suite run it once.
func TestMobilize_AliasedEntriesRunSuiteOnce(t *testing.T) {
	dir := t.TempDir()
	c := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, c)
	catalogTestConfig(t, dir, []string{c.String(), "osps-baseline", "openssf/osps-baseline"})

	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddCatalogs(c.String()); err != nil {
		t.Fatal(err)
	}
	if err := orch.AddEvaluationSuite(c.String(), nil, map[string][]gemara.AssessmentStep{"CCC.Core.C01.TR01": {step_Pass}}); err != nil {
		t.Fatal(err)
	}
	if err := orch.Mobilize(); err != nil {
		t.Fatalf("Mobilize: %v", err)
	}
	if len(orch.Evaluation_Suites) != 1 {
		t.Fatalf("ran %d suites, want 1", len(orch.Evaluation_Suites))
	}
	if got := len(orch.Evaluation_Suites[0].EvaluationLog.Evaluations); got != 1 {
		t.Errorf("evaluated %d controls, want 1 (not re-run)", got)
	}
}

// A cache entry that cannot be read or parsed is a BAD_CATALOG, not a crash.
func TestMobilize_RejectsUnreadableCachedCatalog(t *testing.T) {
	for name, setup := range map[string]func(path string) error{
		"reading": func(path string) error { return os.MkdirAll(path, 0o755) }, // a directory, so ReadFile fails without IsNotExist
		"parsing": func(path string) error { return os.WriteFile(path, []byte("metadata: ["), 0o644) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			c := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
			path := CatalogCachePath(dir, c)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := setup(path); err != nil {
				t.Fatal(err)
			}
			catalogTestConfig(t, dir, []string{c.String()})
			orch := &EvaluationOrchestrator{PluginName: "p"}
			if err := orch.AddCatalogs(c.String()); err != nil {
				t.Fatal(err)
			}
			err := orch.Mobilize()
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected a BAD_CATALOG error mentioning %q, got %v", name, err)
			}
			if got := ExitCodeFor(orch, err); got != shared.BadUsage {
				t.Errorf("exit code = %d, want BadUsage (%d)", got, shared.BadUsage)
			}
		})
	}
}
