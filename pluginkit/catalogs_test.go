package pluginkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/spf13/viper"
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
