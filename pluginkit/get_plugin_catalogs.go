package pluginkit

import (
	"embed"
	"fmt"
	"path"

	"github.com/ossf/gemara/layer2"
	"gopkg.in/yaml.v3"
)

// TODO: When loading the catalogs, queue them all up
// expect one catalog per file, and give each one its own layer2.Catalog
// Then we just need some parent object that contains map of catalogs by Id
// and figure out where we need to pass that around

// GetPluginCatalog reads all YAML catalog files in the data directory and returns the complete catalog data
// This is necessary when packaging the catalog files into a binary, which is not supported by the Gemara loader
// If a catalog imports another, both should be in the same directory
func getPluginCatalogs(dataDir string, files embed.FS) (catalogs []*layer2.Catalog, err error) {
	dir, err := files.ReadDir(dataDir)
	// Check if files are in the right place
	if err != nil || len(dir) == 0 {
		return nil, fmt.Errorf("no contents found in directory: %s", dataDir)
	}

	// Process each YAML file
	for _, file := range dir {
		filePath := path.Join(dataDir, file.Name())
		catalog, err := readYAMLFile(filePath, files)
		if err != nil {
			return nil, err
		}
		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil // just returns the last catalog for now
}

// ReadYAMLFile reads a single YAML file and returns the control family data
func readYAMLFile(filePath string, files embed.FS) (*layer2.Catalog, error) {
	data, err := files.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var catalog layer2.Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return &catalog, nil
}
