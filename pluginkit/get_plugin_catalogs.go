package pluginkit

import (
	"embed"
	"fmt"
	"path"
	"sync"

	"github.com/gemaraproj/go-gemara"
	"github.com/goccy/go-yaml"
)

// TODO: When loading the catalogs, queue them all up
// expect one catalog per file, and give each one its own gemara.ControlCatalog
// Then we just need some parent object that contains map of catalogs by Id
// and figure out where we need to pass that around

// getPluginCatalogs reads all YAML catalog files in the data directory and returns the complete catalog data.
// This is necessary when packaging the catalog files into a binary, which is not supported by the Gemara loader.
// If a catalog imports another, both should be in the same directory.
func getPluginCatalogs(dataDir string, files embed.FS) (catalogs []*gemara.ControlCatalog, err error) {
	dir, err := files.ReadDir(dataDir)
	// Check if files are in the right place
	if err != nil || len(dir) == 0 {
		return nil, fmt.Errorf("no contents found in directory: %s", dataDir)
	}

	catalogs = make([]*gemara.ControlCatalog, len(dir))
	errs := make([]error, len(dir))
	var wg sync.WaitGroup
	for i, file := range dir {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			// This captures pre-RPC crash errors so they don't panic
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("parsing catalog %q: %v", name, r)
				}
			}()
			catalogs[i], errs[i] = readYAMLFile(path.Join(dataDir, name), files)
		}(i, file.Name())
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return catalogs, nil
}

// readYAMLFile reads a single YAML file and returns the control family data.
func readYAMLFile(filePath string, files embed.FS) (*gemara.ControlCatalog, error) {
	data, err := files.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var catalog gemara.ControlCatalog
	// Use goccy/go-yaml (not gopkg.in/yaml.v3) so gemara enum types like EntityType
	// are unmarshaled via their UnmarshalYAML([]byte) methods.
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML %s: %w", filePath, err)
	}

	return &catalog, nil
}
