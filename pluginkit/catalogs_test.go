package pluginkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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

// embeddedCatalogFS is a plugin's embedded copy of the catalog at c, as
// AddReferenceCatalogs reads it: keyed by metadata id, carrying no version.
func embeddedCatalogFS(id string) fstest.MapFS {
	yaml := "metadata:\n  id: " + id + "\ncontrols:\n" +
		"  - id: CCC.Core.C01\n    title: T\n    objective: O\n    assessment-requirements:\n" +
		"      - id: CCC.Core.C01.TR01\n        text: t\n        applicability: [tlp-green]\n"
	return fstest.MapFS{"data/" + id + ".yaml": &fstest.MapFile{Data: []byte(yaml)}}
}

// A plugin migrating incrementally declares a catalog and still embeds it. The
// verified copy must win: the embedded one was never checked against grc.store.
func TestMatchSuite_DeclaredCoordinateBeatsEmbeddedCopy(t *testing.T) {
	dir := t.TempDir()
	c := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, c)
	catalogTestConfig(t, dir, []string{"osps-baseline"}) // the bare metadata id matches both

	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddReferenceCatalogs("data", embeddedCatalogFS(c.ID)); err != nil {
		t.Fatal(err)
	}
	if err := orch.AddCatalogs(c.String()); err != nil {
		t.Fatal(err)
	}
	steps := map[string][]gemara.AssessmentStep{"CCC.Core.C01.TR01": {step_Pass}}
	if err := orch.AddEvaluationSuiteForAllCatalogs(nil, steps); err != nil {
		t.Fatal(err)
	}
	if err := orch.Mobilize(); err != nil {
		t.Fatalf("Mobilize: %v", err)
	}

	// Both suites are registered; the test is about which one the bare id picks.
	if len(orch.possibleSuites) != 2 {
		t.Fatalf("registered %d suites, want the embedded one and the declared one", len(orch.possibleSuites))
	}
	if len(orch.Evaluation_Suites) != 1 || orch.Evaluation_Suites[0].CatalogId != c.String() {
		t.Fatalf("ran %+v, want just the declared coordinate %s", orch.Evaluation_Suites, c)
	}
	// With nothing declared, the embedded copy is still matched.
	plain := &EvaluationOrchestrator{PluginName: "p"}
	if err := plain.AddReferenceCatalogs("data", embeddedCatalogFS(c.ID)); err != nil {
		t.Fatal(err)
	}
	if err := plain.AddEvaluationSuiteForAllCatalogs(nil, steps); err != nil {
		t.Fatal(err)
	}
	if got := plain.matchSuite(c.ID); got == nil || got.CatalogId != c.ID {
		t.Errorf("an embedded catalog with no declared rival must still match, got %v", got)
	}
}

// A suite that cannot run must not report a passing run. Mobilize logs the
// error rather than returning it, so the outcome has to live on the suite.
func TestMobilize_SuiteThatCannotRunFailsTheRun(t *testing.T) {
	dir := t.TempDir()
	c := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, c)
	catalogTestConfig(t, dir, []string{c.String()})

	orch := &EvaluationOrchestrator{PluginName: "p"}
	if err := orch.AddCatalogs(c.String()); err != nil {
		t.Fatal(err)
	}
	// No steps: Evaluate fails with NO_ASSESSMENT_STEPS_PROVIDED before it
	// assesses anything, so the suite's own Result is never set by the run.
	if err := orch.AddEvaluationSuite(c.String(), nil, nil); err != nil {
		t.Fatal(err)
	}
	err := orch.Mobilize()
	if err != nil {
		t.Fatalf("Mobilize reports a failed suite through the results, not an error: %v", err)
	}
	if len(orch.Evaluation_Suites) != 1 || orch.Evaluation_Suites[0].Result != gemara.Unknown {
		t.Fatalf("suite result = %v, want Unknown", orch.Evaluation_Suites)
	}
	if got := ExitCodeFor(orch, err); got != shared.TestFail {
		t.Errorf("exit code = %d, want TestFail (%d)", got, shared.TestFail)
	}
}

func TestAddCatalogs_MobilizeLoadsFromCacheAndMatchesConfig(t *testing.T) {
	dir := t.TempDir()
	v2 := CatalogCoordinate{"openssf", "osps-baseline", "v2"}
	v1 := CatalogCoordinate{"openssf", "osps-baseline", "v1"}
	cachedCatalog(t, dir, v2)
	cachedCatalog(t, dir, v1)
	// A bare coordinate resolves to the newest declared version, whatever the
	// declaration order.
	catalogTestConfig(t, dir, []string{"openssf/osps-baseline"})

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
	if got := orch.matchSuite("osps-baseline"); got == nil || got.CatalogId != v2.String() {
		t.Errorf("metadata id should resolve to the newest declared version, got %v", got)
	}
	if got := orch.matchSuite(v1.String()); got == nil || got.CatalogId != v1.String() {
		t.Errorf("exact coordinate should resolve to v1, got %v", got)
	}
	if orch.matchSuite("nope") != nil {
		t.Error("unknown catalog must not match")
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

func TestCompareCatalogVersions(t *testing.T) {
	newer := [][2]string{
		{"v2026.08.26", "v2026.02.17"},
		{"v1.10", "v1.9"},
		{"2.0", "v1.9.9"},
		{"v1.0.1", "v1.0"},
		{"v1.0-rc2", "v1.0-rc1"},
	}
	for _, p := range newer {
		if compareCatalogVersions(p[0], p[1]) <= 0 || compareCatalogVersions(p[1], p[0]) >= 0 {
			t.Errorf("%s should be newer than %s", p[0], p[1])
		}
	}
	if compareCatalogVersions("v1.2", "1.2") != 0 {
		t.Error("a leading v must not matter")
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
