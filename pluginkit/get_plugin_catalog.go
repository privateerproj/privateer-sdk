package pluginkit

import (
	"embed"
	"fmt"
	"path"

	"github.com/ossf/gemara/layer2"
	"gopkg.in/yaml.v3"
)

// GetCatalog reads all YAML catalog files in the data directory and returns the complete catalog data
// This is necessary when packaging the catalog files into a binary, which is not supported by the Gemara loader
func GetPluginCatalog(dataDir string, files embed.FS) (catalog layer2.Catalog, err error) {
	dir, err := files.ReadDir(dataDir)
	// Check if files are in the right place
	if err != nil {
		return catalog, fmt.Errorf("data directory does not exist: %s", dataDir)
	}

	catalog = layer2.Catalog{
		ControlFamilies: []layer2.ControlFamily{},
	}

	// Process each YAML file
	for _, file := range dir {
		filePath := path.Join(dataDir, file.Name())
		data, err := readYAMLFile(filePath, files)
		if err != nil {
			return catalog, err
		}
		if len(data.Controls) == 0 {
			continue // not a controls file, skip
		}
		catalog.ControlFamilies = append(catalog.ControlFamilies, *data)
	}

	return catalog, nil
}

// ReadYAMLFile reads a single YAML file and returns the control family data
func readYAMLFile(filePath string, files embed.FS) (*layer2.ControlFamily, error) {
	data, err := files.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var yamlData layer2.Catalog
	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if len(yamlData.ControlFamilies) == 0 {
		return &layer2.ControlFamily{}, nil // not a controls file
	}

	// Assuming one control family per file as per the current structure
	familyData := yamlData.ControlFamilies[0]

	controlFamily := &layer2.ControlFamily{
		Id:          familyData.Id, // Use the ID from the YAML data
		Title:       familyData.Title,
		Description: familyData.Description,
		Controls:    familyData.Controls,
	}

	return controlFamily, nil
}
