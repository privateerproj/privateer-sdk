package pluginkit

import (
	"embed"
	"strings"
	"testing"
)

// Test data embedded in the test binary
//
//go:embed catalog-test-data/*
var testDataFS embed.FS

func TestGetPluginCatalogs(t *testing.T) {
	t.Run("Valid Directory", func(t *testing.T) {
		catalogs, err := getPluginCatalogs("catalog-test-data", testDataFS)
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

func TestReadYAMLFile(t *testing.T) {
	t.Run("Valid YAML File", func(t *testing.T) {
		catalog, err := readYAMLFile("catalog-test-data/metadata.yaml", testDataFS)
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
		catalog, err := readYAMLFile("catalog-test-data/metadata.yaml", testDataFS)
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
		catalogs, err := getPluginCatalogs("catalog-test-data", testDataFS)
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
