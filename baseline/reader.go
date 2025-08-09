package baseline

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/revanite-io/sci/layer2"
)

const path string = "data"

//go:embed data
var files embed.FS

// BaselineData represents the complete baseline data structure
type BaselineData struct {
	// ControlFamilyFiles maps family ID to the file path it was loaded from
	ControlFamilyFiles map[string]string `json:"control_family_files"`
	// Catalog contains all the control families and their controls
	Catalog layer2.Catalog `json:"catalog"`
}

// ControlFamilyData represents a single YAML file structure
type ControlFamilyData struct {
	Title       string           `yaml:"title"`
	Description string           `yaml:"description"`
	Controls    []layer2.Control `yaml:"controls"`
}

// Reader provides functionality to read baseline YAML files
type Reader struct {
	dataDir string
}

// NewReader creates a new Reader instance
func NewReader(dataDir string) *Reader {
	return &Reader{
		dataDir: dataDir,
	}
}

// ReadAllYAMLFiles reads all YAML files in the data directory and returns the complete baseline data
func (r *Reader) ReadAllYAMLFiles() (*BaselineData, error) {

	dir,_ := files.ReadDir(path)
	for _,file := range dir{
		fmt.Println("File: ", file.Name())
	}

	baselineData := &BaselineData{
		ControlFamilyFiles: make(map[string]string),
		Catalog: layer2.Catalog{
			ControlFamilies: []layer2.ControlFamily{},
		},
	}

	// Check if files are in the right place
	if _, err := os.Stat(r.dataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("data directory does not exist: %s", r.dataDir)
	}

	// Process each YAML file
	for _, filePath := range dir {
		controlFamily, familyID, err := r.readYAMLFile(filePath.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		baselineData.ControlFamilyFiles[familyID] = filePath.Name()
		baselineData.Catalog.ControlFamilies = append(baselineData.Catalog.ControlFamilies, *controlFamily)
	}

	return baselineData, nil
}

// ReadYAMLFile reads a single YAML file and returns the control family data
func (r *Reader) ReadYAMLFile(filename string) (*layer2.ControlFamily, error) {
	filePath := filepath.Join(r.dataDir, filename)
	controlFamily, _, err := r.readYAMLFile(filePath)
	return controlFamily, err
}

// ListYAMLFiles returns a list of all YAML files in the data directory
func (r *Reader) ListYAMLFiles() ([]string, error) {
	files, err := r.findYAMLFiles()
	if err != nil {
		return nil, err
	}

	// Return just the filenames, not full paths
	var filenames []string
	for _, file := range files {
		filenames = append(filenames, filepath.Base(file))
	}

	return filenames, nil
}

// GetControlFamilyCount returns the number of control families available
func (r *Reader) GetControlFamilyCount() (int, error) {
	files, err := r.findYAMLFiles()
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

// findYAMLFiles discovers all YAML files in the data directory
func (r *Reader) findYAMLFiles() ([]string, error) {
	var yamlFiles []string

	err := filepath.WalkDir(r.dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Check for .yaml or .yml extensions
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			yamlFiles = append(yamlFiles, path)
		}

		return nil
	})

	return yamlFiles, err
}

// readYAMLFile reads and parses a single YAML file
func (r *Reader) readYAMLFile(filePath string) (*layer2.ControlFamily, string, error) {
	data, err := os.ReadFile(path+"/"+filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	var familyData ControlFamilyData
	if err := yaml.Unmarshal(data, &familyData); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	controlFamily := &layer2.ControlFamily{
		Title:       familyData.Title,
		Description: familyData.Description,
		Controls:    familyData.Controls,
	}

	// Extract family ID from filename (e.g., "OSPS-AC.yaml" -> "AC")
	filename := filepath.Base(filePath)
	familyID := r.extractFamilyID(filename)

	return controlFamily, familyID, nil
}

// extractFamilyID extracts the family ID from a filename
// e.g., "OSPS-AC.yaml" -> "AC"
func (r *Reader) extractFamilyID(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	
	// Handle OSPS-XX pattern
	if strings.HasPrefix(name, "OSPS-") && len(name) > 5 {
		return name[5:] // Return everything after "OSPS-"
	}
	
	// Fallback to the full name without extension
	return name
}

// GetControlByID searches for a control by its ID across all control families
func (r *Reader) GetControlByID(controlID string) (*layer2.Control, string, error) {
	baselineData, err := r.ReadAllYAMLFiles()
	if err != nil {
		return nil, "", err
	}

	for _, family := range baselineData.Catalog.ControlFamilies {
		for _, control := range family.Controls {
			if control.Id == controlID {
				return &control, family.Title, nil
			}
		}
	}

	return nil, "", fmt.Errorf("control with ID %s not found", controlID)
}

func(r *Reader)GetAssesmentRequirementById(assessmentID string)(*layer2.AssessmentRequirement, error){
	//extract the control id
	controlID := strings.Split(assessmentID, ".")[0]
	control,_,err := r.GetControlByID(controlID)
	
	if ( err != nil ){
		return nil, err
	}

	for _,assessment := range control.AssessmentRequirements {
		if(assessment.Id == assessmentID ){
			return &assessment, nil
		}
	}

	return nil, fmt.Errorf("control with ID %s not found", controlID)
}

// GetControlsByFamily returns all controls for a specific family
func (r *Reader) GetControlsByFamily(familyID string) ([]layer2.Control, error) {
	filename := fmt.Sprintf("OSPS-%s.yaml", familyID)
	controlFamily, err := r.ReadYAMLFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read family %s: %w", familyID, err)
	}

	return controlFamily.Controls, nil
}

// PrintSummary prints a summary of all loaded baseline data
func (r *Reader) PrintSummary() error {
	baselineData, err := r.ReadAllYAMLFiles()
	if err != nil {
		return err
	}

	fmt.Printf("Baseline Data Summary:\n")
	fmt.Printf("=====================\n")
	fmt.Printf("Total Control Families: %d\n\n", len(baselineData.Catalog.ControlFamilies))

	totalControls := 0
	totalRequirements := 0

	for _, family := range baselineData.Catalog.ControlFamilies {
		controlCount := len(family.Controls)
		totalControls += controlCount

		requirementCount := 0
		for _, control := range family.Controls {
			requirementCount += len(control.AssessmentRequirements)
		}
		totalRequirements += requirementCount

		fmt.Printf("Family: %s\n", family.Title)
		fmt.Printf("  Description: %s\n", family.Description)
		fmt.Printf("  Controls: %d\n", controlCount)
		fmt.Printf("  Assessment Requirements: %d\n\n", requirementCount)
	}

	fmt.Printf("Totals:\n")
	fmt.Printf("  Controls: %d\n", totalControls)
	fmt.Printf("  Assessment Requirements: %d\n", totalRequirements)

	return nil
}
