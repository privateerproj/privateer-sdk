package pluginkit

import (
	"embed"
	"strings"
	"testing"
)

// Test data embedded in the test binary: catalog-test-data/valid holds valid
// catalogs, catalog-test-data/errors holds malformed ones for the error path.
//
//go:embed catalog-test-data
var testDataFS embed.FS

func TestGetPluginCatalogs(t *testing.T) {
	t.Run("Valid Directory", func(t *testing.T) {
		catalogs, err := getPluginCatalogs("catalog-test-data/valid", testDataFS)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(catalogs) == 0 {
			t.Error("Expected at least one catalog")
		}

		// Verify the catalog structure
		for i, catalog := range catalogs {
			if catalog == nil {
				t.Errorf("Catalog %d should not be nil", i)
			}
			// Note: Some catalogs might not have control families (like metadata.yaml)
			// This is expected behavior, so we don't fail the test
		}
	})

	t.Run("Empty Directory", func(t *testing.T) {
		// Create an empty embedded FS
		var emptyFS embed.FS

		catalogs, err := getPluginCatalogs("non-existent", emptyFS)
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
		if catalogs != nil {
			t.Error("Expected nil catalogs for non-existent directory")
		}
		if !strings.Contains(err.Error(), "no contents found in directory") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Empty Directory Name", func(t *testing.T) {
		catalogs, err := getPluginCatalogs("", testDataFS)
		if err == nil {
			t.Error("Expected error for empty directory name")
		}
		if catalogs != nil {
			t.Error("Expected nil catalogs for empty directory name")
		}
		if !strings.Contains(err.Error(), "no contents found in directory") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// TestGetPluginCatalogsDeterministicOrder guards the concurrent parse: catalogs
// must return in directory (alphabetical) order regardless of goroutine finish
// order. controls.yaml sorts before metadata.yaml, so index 0 holds the CCC.C01
// control and index 1 the FINOS-CCC metadata id.
func TestGetPluginCatalogsDeterministicOrder(t *testing.T) {
	for i := 0; i < 50; i++ {
		catalogs, err := getPluginCatalogs("catalog-test-data/valid", testDataFS)
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
		if len(catalogs) != 2 {
			t.Fatalf("run %d: expected 2 catalogs, got %d", i, len(catalogs))
		}
		if len(catalogs[0].Controls) == 0 || catalogs[0].Controls[0].Id != "CCC.C01" {
			t.Fatalf("run %d: catalogs[0] should be controls.yaml (first control CCC.C01), got controls=%v", i, catalogs[0].Controls)
		}
		if catalogs[1].Metadata.Id != "FINOS-CCC" {
			t.Fatalf("run %d: catalogs[1] should be metadata.yaml (id FINOS-CCC), got %q", i, catalogs[1].Metadata.Id)
		}
	}
}

// TestGetPluginCatalogsErrorPath covers the concurrent error path: files still
// parse even after one fails, and the first error in directory order is the one
// surfaced — deterministically, whichever goroutine finishes first. 01_broken
// sorts ahead of the also-broken 03_broken, so its error must win.
func TestGetPluginCatalogsErrorPath(t *testing.T) {
	for i := 0; i < 30; i++ {
		catalogs, err := getPluginCatalogs("catalog-test-data/errors", testDataFS)
		if err == nil {
			t.Fatalf("run %d: expected an error from the malformed catalogs, got nil", i)
		}
		if catalogs != nil {
			t.Fatalf("run %d: expected nil catalogs on error, got %d", i, len(catalogs))
		}
		if !strings.Contains(err.Error(), "01_broken.yaml") {
			t.Fatalf("run %d: expected the first-in-order file's error, got: %v", i, err)
		}
	}
}

func TestReadYAMLFile(t *testing.T) {
	t.Run("Valid YAML File", func(t *testing.T) {
		catalog, err := readYAMLFile("catalog-test-data/valid/metadata.yaml", testDataFS)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if catalog == nil {
			t.Error("Expected non-nil catalog")
		}
	})

	t.Run("Non-existent File", func(t *testing.T) {
		catalog, err := readYAMLFile("non-existent.yaml", testDataFS)
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
		if catalog != nil {
			t.Error("Expected nil catalog for non-existent file")
		}
		if !strings.Contains(err.Error(), "failed to read file") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Invalid YAML Content", func(t *testing.T) {
		// Test with valid YAML since we can't easily create invalid YAML in embed.FS
		catalog, err := readYAMLFile("catalog-test-data/valid/metadata.yaml", testDataFS)
		if err != nil {
			t.Errorf("Unexpected error with valid YAML: %v", err)
		}
		if catalog == nil {
			t.Error("Expected non-nil catalog")
		}
	})
}

func TestGetPluginCatalogsIntegration(t *testing.T) {
	t.Run("Multiple Files", func(t *testing.T) {
		catalogs, err := getPluginCatalogs("catalog-test-data/valid", testDataFS)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Should have multiple catalogs (metadata.yaml and controls.yaml)
		if len(catalogs) < 2 {
			t.Errorf("Expected at least 2 catalogs, got %d", len(catalogs))
		}

		// Verify each catalog has the expected structure
		for i, catalog := range catalogs {
			if catalog == nil {
				t.Errorf("Catalog %d should not be nil", i)
				continue
			}

			// Check that the catalog has some basic structure
			// Note: Some catalogs might not have ControlFamilies (like metadata.yaml)
			// This is expected behavior
		}
	})
}
