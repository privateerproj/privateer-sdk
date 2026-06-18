package pluginkit

import (
	"reflect"
	"strings"
	"testing"

	gemara "github.com/gemaraproj/go-gemara"
)

// orchestratorWithCatalog builds an orchestrator carrying one reference catalog
// and a PluginName, bypassing AddReferenceCatalogs' embed.FS plumbing (this
// package's tests can set the unexported field directly). The Publisher and the
// catalog's author are left to the caller so fail-closed cases can omit them.
func orchestratorWithCatalog(id, version string, controlIDs ...string) *EvaluationOrchestrator {
	controls := make([]gemara.Control, 0, len(controlIDs))
	for _, cid := range controlIDs {
		controls = append(controls, gemara.Control{Id: cid})
	}
	return &EvaluationOrchestrator{
		PluginName: "hello",
		License:    "Apache-2.0",
		referenceCatalogs: map[string]*gemara.ControlCatalog{
			id: {
				Metadata: gemara.Metadata{Id: id, Version: version},
				Controls: controls,
			},
		},
	}
}

// setCatalogAuthor sets a reference catalog's metadata.author.id, the source of
// truth for the catalog's owning namespace in the publish manifest.
func setCatalogAuthor(orch *EvaluationOrchestrator, catalogID, authorID string) {
	c := orch.referenceCatalogs[catalogID]
	c.Metadata.Author = gemara.Actor{Id: authorID}
}

func TestPublishManifest_DerivesEverythingFromFieldsAndCatalog(t *testing.T) {
	orch := orchestratorWithCatalog("ccc.build.cn", "2026.04", "CCC.Build.C02", "CCC.Build.C01", "CCC.Build.C01")
	orch.Publisher = "acme"
	// The plugin author also owns the catalog it evaluates: author.id == publisher.
	setCatalogAuthor(orch, "ccc.build.cn", "acme")

	m, err := orch.PublishManifest()
	if err != nil {
		t.Fatalf("PublishManifest: %v", err)
	}
	// coordinate = Publisher + "/" + PluginName.
	if m.Coordinate != "acme/hello" {
		t.Errorf("coordinate = %q, want acme/hello", m.Coordinate)
	}
	if len(m.Evaluates) != 1 {
		t.Fatalf("expected 1 evaluates entry, got %d", len(m.Evaluates))
	}
	e := m.Evaluates[0]
	// catalog namespace = the catalog's own author.id; id + version from the catalog.
	if e.Catalog != "acme/ccc.build.cn" {
		t.Errorf("catalog = %q, want acme/ccc.build.cn", e.Catalog)
	}
	if e.CatalogVersion != "2026.04" {
		t.Errorf("catalog_version = %q", e.CatalogVersion)
	}
	// Deduplicated and sorted (deterministic for the signed config blob).
	want := []string{"CCC.Build.C01", "CCC.Build.C02"}
	if len(e.RequirementIDs) != len(want) || e.RequirementIDs[0] != want[0] || e.RequirementIDs[1] != want[1] {
		t.Errorf("requirement_ids = %v, want %v (deduped + sorted)", e.RequirementIDs, want)
	}
}

func TestPublishManifest_FailsClosed(t *testing.T) {
	t.Run("no publisher", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "1", "R1") // PluginName set, Publisher not
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "Publisher") {
			t.Fatalf("expected a Publisher error, got %v", err)
		}
	})
	t.Run("no plugin name", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "1", "R1")
		orch.PluginName = ""
		orch.Publisher = "acme"
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "PluginName") {
			t.Fatalf("expected a PluginName error, got %v", err)
		}
	})
	t.Run("no reference catalogs", func(t *testing.T) {
		orch := &EvaluationOrchestrator{Publisher: "acme", PluginName: "hello", License: "Apache-2.0"}
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "no reference catalogs") {
			t.Fatalf("expected a no-catalogs error, got %v", err)
		}
	})
	t.Run("catalog with no version", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "", "R1") // empty metadata.version
		orch.Publisher = "acme"
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "metadata.version") {
			t.Fatalf("expected a metadata.version error, got %v", err)
		}
	})
	t.Run("catalog with no controls", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "1") // no controls
		orch.Publisher = "acme"
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "no controls") {
			t.Fatalf("expected a no-controls error, got %v", err)
		}
	})
	t.Run("catalog with no author id and no override", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "1", "R1") // version + controls, but no author, no override
		orch.Publisher = "acme"
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "author.id") {
			t.Fatalf("expected an author.id error (no fabricated namespace), got %v", err)
		}
	})
	t.Run("no license", func(t *testing.T) {
		orch := orchestratorWithCatalog("c", "1", "R1")
		orch.Publisher = "acme"
		setCatalogAuthor(orch, "c", "acme")
		orch.License = "" // otherwise publishable, but grc.store requires a license
		if _, err := orch.PublishManifest(); err == nil || !strings.Contains(err.Error(), "License") {
			t.Fatalf("expected a License error, got %v", err)
		}
	})
}

// orchestratorWithImportingCatalog builds an orchestrator with two reference
// catalogs: "primary" imports one control from "imported".  The importing
// catalog has its own control P-1 plus one imported control I-1.
func orchestratorWithImportingCatalog() *EvaluationOrchestrator {
	primary := &gemara.ControlCatalog{
		Metadata: gemara.Metadata{Id: "primary", Version: "1.0", Author: gemara.Actor{Id: "acme"}},
		Controls: []gemara.Control{{Id: "P-1"}},
		Imports: []gemara.MultiEntryMapping{
			{
				ReferenceId: "imported",
				Entries:     []gemara.ArtifactMapping{{ReferenceId: "I-1"}},
			},
		},
	}
	imported := &gemara.ControlCatalog{
		Metadata: gemara.Metadata{Id: "imported", Version: "2.0", Author: gemara.Actor{Id: "acme"}},
		Controls: []gemara.Control{{Id: "I-1"}, {Id: "I-2"}},
	}
	return &EvaluationOrchestrator{
		PluginName: "hello",
		Publisher:  "acme",
		License:    "Apache-2.0",
		referenceCatalogs: map[string]*gemara.ControlCatalog{
			"primary":  primary,
			"imported": imported,
		},
	}
}

// TestPublishManifest_CopyOnImport verifies that AddEvaluationSuite does not
// mutate the shared referenceCatalogs entry: PublishManifest output must be
// byte-for-byte identical whether called before or after suite registration,
// and the importing catalog's RequirementIDs must list only its OWN control
// ids (not the ones it imports from another catalog).
func TestPublishManifest_CopyOnImport(t *testing.T) {
	orch := orchestratorWithImportingCatalog()

	// Capture manifest BEFORE suite registration.
	before, err := orch.PublishManifest()
	if err != nil {
		t.Fatalf("PublishManifest before AddEvaluationSuite: %v", err)
	}

	// Register a suite for the importing catalog (this triggers getImportedControls).
	steps := map[string][]gemara.AssessmentStep{"P-1": {step_Pass}}
	if err := orch.AddEvaluationSuite("primary", nil, steps); err != nil {
		t.Fatalf("AddEvaluationSuite: %v", err)
	}

	// Capture manifest AFTER suite registration.
	after, err := orch.PublishManifest()
	if err != nil {
		t.Fatalf("PublishManifest after AddEvaluationSuite: %v", err)
	}

	// The two manifests must be identical — referenceCatalogs was not mutated.
	if !reflect.DeepEqual(before, after) {
		t.Errorf("PublishManifest changed after AddEvaluationSuite:\nbefore=%+v\nafter=%+v", before, after)
	}

	// The importing catalog's RequirementIDs must contain only its OWN control.
	var primaryEntry *EvaluatesDeclaration
	for i := range after.Evaluates {
		if after.Evaluates[i].Catalog == "acme/primary" {
			primaryEntry = &after.Evaluates[i]
			break
		}
	}
	if primaryEntry == nil {
		t.Fatalf("no evaluates entry for acme/primary in manifest: %+v", after.Evaluates)
	}
	if len(primaryEntry.RequirementIDs) != 1 || primaryEntry.RequirementIDs[0] != "P-1" {
		t.Errorf("primary catalog RequirementIDs = %v, want [P-1] (own controls only, not imports)", primaryEntry.RequirementIDs)
	}
}

// TestPublishManifest_CatalogNamespaces verifies the namespace resolution:
// the catalog's own author.id is the default source of truth, an explicit
// CatalogNamespaces entry overrides it, and invalid overrides (empty or
// containing "/") are rejected. The plugin's Publisher is NEVER used as a
// catalog namespace.
func TestPublishManifest_CatalogNamespaces(t *testing.T) {
	t.Run("override wins over author.id", func(t *testing.T) {
		orch := orchestratorWithCatalog("osps-baseline", "2025.01", "OSPS-AC-01")
		orch.Publisher = "communityorg"
		setCatalogAuthor(orch, "osps-baseline", "openssf")
		orch.CatalogNamespaces = map[string]string{"osps-baseline": "ossf-mirror"}

		m, err := orch.PublishManifest()
		if err != nil {
			t.Fatalf("PublishManifest: %v", err)
		}
		if len(m.Evaluates) != 1 {
			t.Fatalf("expected 1 evaluates entry, got %d", len(m.Evaluates))
		}
		if m.Evaluates[0].Catalog != "ossf-mirror/osps-baseline" {
			t.Errorf("catalog = %q, want ossf-mirror/osps-baseline", m.Evaluates[0].Catalog)
		}
	})

	t.Run("uses catalog author.id, not the plugin publisher", func(t *testing.T) {
		// A community plugin (publisher "communityorg") evaluating OpenSSF's
		// catalog must link to openssf/..., NOT communityorg/...
		orch := orchestratorWithCatalog("osps-baseline", "2025.10", "OSPS-AC-01")
		orch.Publisher = "communityorg"
		setCatalogAuthor(orch, "osps-baseline", "openssf")

		m, err := orch.PublishManifest()
		if err != nil {
			t.Fatalf("PublishManifest: %v", err)
		}
		if m.Evaluates[0].Catalog != "openssf/osps-baseline" {
			t.Errorf("catalog = %q, want openssf/osps-baseline (author, not publisher)", m.Evaluates[0].Catalog)
		}
	})

	t.Run("invalid override containing slash errors", func(t *testing.T) {
		orch := orchestratorWithCatalog("some-catalog", "1.0", "REQ-1")
		orch.Publisher = "acme"
		orch.CatalogNamespaces = map[string]string{"some-catalog": "bad/namespace"}

		_, err := orch.PublishManifest()
		if err == nil {
			t.Fatal("expected error for override containing '/', got nil")
		}
		if !strings.Contains(err.Error(), "not a valid namespace") {
			t.Errorf("expected 'not a valid namespace' error, got: %v", err)
		}
	})

	t.Run("invalid override that is empty string errors", func(t *testing.T) {
		orch := orchestratorWithCatalog("some-catalog", "1.0", "REQ-1")
		orch.Publisher = "acme"
		orch.CatalogNamespaces = map[string]string{"some-catalog": "   "}

		_, err := orch.PublishManifest()
		if err == nil {
			t.Fatal("expected error for empty override, got nil")
		}
		if !strings.Contains(err.Error(), "not a valid namespace") {
			t.Errorf("expected 'not a valid namespace' error, got: %v", err)
		}
	})
}

// TestAddEvaluationSuite_ImportedControlsStillEvaluated verifies that the
// copy-on-import change does not change evaluation behavior: a suite for a
// catalog that imports controls from another catalog still evaluates the
// imported controls.
func TestAddEvaluationSuite_ImportedControlsStillEvaluated(t *testing.T) {
	orch := orchestratorWithImportingCatalog()

	// Register steps covering both the own control and the imported one.
	steps := map[string][]gemara.AssessmentStep{
		"P-1": {step_Pass},
		"I-1": {step_Pass},
	}
	if err := orch.AddEvaluationSuite("primary", nil, steps); err != nil {
		t.Fatalf("AddEvaluationSuite: %v", err)
	}

	if len(orch.possibleSuites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(orch.possibleSuites))
	}
	suite := orch.possibleSuites[0]

	// The suite's catalog must contain both the own control and the imported one.
	controlIDs := make(map[string]bool)
	for _, c := range suite.catalog.Controls {
		controlIDs[c.Id] = true
	}
	if !controlIDs["P-1"] {
		t.Errorf("suite catalog missing own control P-1; controls = %v", suite.catalog.Controls)
	}
	if !controlIDs["I-1"] {
		t.Errorf("suite catalog missing imported control I-1; controls = %v", suite.catalog.Controls)
	}
}
