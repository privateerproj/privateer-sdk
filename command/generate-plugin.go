package command

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/utils"
)

// CatalogData extends gemara.ControlCatalog with additional fields for plugin generation.
type CatalogData struct {
	gemara.ControlCatalog
	ServiceName             string
	Requirements            []Req
	ApplicabilityCategories []string
	StrippedName            string
}

// Req represents an assessment requirement with an ID and text description.
type Req struct {
	Id   string
	Text string
}

// GeneratePlugin generates a plugin from a catalog file.
func GeneratePlugin(logger hclog.Logger, templatesDir, sourcePath, outputDir, serviceName string) error {
	data := CatalogData{}
	data.ServiceName = serviceName

	err := data.LoadFile("file://" + sourcePath)
	if err != nil {
		return err
	}

	err = data.getAssessmentRequirements()
	if err != nil {
		return err
	}

	err = filepath.Walk(templatesDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				err = generateFileFromTemplate(data, path, templatesDir, outputDir, logger)
				if err != nil {
					logger.Error(fmt.Sprintf("Failed while writing in dir '%s': %s", outputDir, err))
				}
			} else if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("error walking through templates directory: %w", err)
	}

	err = writeCatalogFile(&data.ControlCatalog, outputDir)
	if err != nil {
		return fmt.Errorf("failed to write catalog to file: %w", err)
	}

	return nil
}

// SetupTemplatingEnvironment validates and sets up the environment for plugin generation.
func SetupTemplatingEnvironment(logger hclog.Logger) (templatesDir, sourcePath, outputDir, serviceName string, err error) {
	sourcePath = viper.GetString("source-path")
	if sourcePath == "" {
		return "", "", "", "", fmt.Errorf("required: --source-path is required to generate a plugin from a control set from local file or URL")
	}

	serviceName = viper.GetString("service-name")
	if serviceName == "" {
		return "", "", "", "", fmt.Errorf("required: --service-name is required to generate a plugin")
	}

	if viper.GetString("local-templates") != "" {
		templatesDir = viper.GetString("local-templates")
	} else {
		templatesDir = filepath.Join(os.TempDir(), "privateer-templates")
		err = setupTemplatesDir(templatesDir, logger)
		if err != nil {
			return "", "", "", "", fmt.Errorf("error setting up templates directory: %w", err)
		}
	}

	outputDir = viper.GetString("output-dir")
	logger.Trace(fmt.Sprintf("Generated plugin will be stored in this directory: %s", outputDir))

	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return "", "", "", "", err
	}

	return templatesDir, sourcePath, outputDir, serviceName, nil
}

func setupTemplatesDir(templatesDir string, logger hclog.Logger) error {
	// Remove any old templates
	err := os.RemoveAll(templatesDir)
	if err != nil {
		logger.Error("Failed to remove templates directory: %s", err)
	}

	// Pull latest templates from git
	logger.Trace(fmt.Sprintf("Cloning templates repo to: %s", templatesDir))
	_, err = git.PlainClone(templatesDir, false, &git.CloneOptions{
		URL:      "https://github.com/privateerproj/plugin-generator-templates.git",
		Progress: os.Stdout,
	})
	return err
}

func generateFileFromTemplate(data CatalogData, templatePath, templatesDir, outputDir string, logger hclog.Logger) error {
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("error reading template file %s: %w", templatePath, err)
	}

	// Determine relative path from templates dir so we can preserve subdirs in output
	relativeFilepath, err := filepath.Rel(templatesDir, templatePath)
	if err != nil {
		return fmt.Errorf("error calculating relative path for %s: %w", templatePath, err)
	}

	// If the template is not a text template, copy it over as-is (preserve mode)
	if filepath.Ext(templatePath) != ".txt" {
		return copyNonTemplateFile(templatePath, relativeFilepath, outputDir, logger)
	}

	tmpl, err := template.New("plugin").Funcs(template.FuncMap{
		"as_text": func(in string) template.HTML {
			return template.HTML(
				strings.TrimSpace(
					strings.ReplaceAll(in, "\n", " ")))
		},
		"default": func(in string, out string) string {
			if in != "" {
				return in
			}
			return out
		},
		"snake_case": snakeCase,
	}).Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("error parsing template file %s: %w", templatePath, err)
	}

	outputPath := filepath.Join(outputDir, strings.TrimSuffix(relativeFilepath, ".txt"))

	err = os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating directories for %s: %w", outputPath, err)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file %s: %w", outputPath, err)
	}

	defer func() {
		err := outputFile.Close()
		if err != nil {
			logger.Error("error closing output file %s: %w", outputPath, err)
		}
	}()

	err = tmpl.Execute(outputFile, data)
	if err != nil {
		return fmt.Errorf("error executing template for file %s: %w", outputPath, err)
	}

	return nil
}

func (c *CatalogData) getAssessmentRequirements() error {
	for _, control := range c.Controls {
		for _, requirement := range control.AssessmentRequirements {
			req := Req{
				Id:   requirement.Id,
				Text: requirement.Text,
			}
			c.Requirements = append(c.Requirements, req)
			// Add applicability categories if unique
			for _, a := range requirement.Applicability {
				if !utils.StringSliceContains(c.ApplicabilityCategories, a) {
					c.ApplicabilityCategories = append(c.ApplicabilityCategories, a)
				}
			}
		}
	}
	if len(c.Requirements) == 0 {
		return errors.New("no requirements retrieved from catalog")
	}
	return nil
}

func writeCatalogFile(catalog *gemara.ControlCatalog, outputDir string) error {
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2) // this is the line that sets the indentation
	err := yamlEncoder.Encode(catalog)
	if err != nil {
		return fmt.Errorf("error marshaling YAML: %w", err)
	}

	dirPath := filepath.Join(outputDir, "data", "catalogs")
	id := snakeCase(catalog.Metadata.Id)
	version := snakeCase(catalog.Metadata.Version)
	fileName := fmt.Sprintf("catalog_%s_%s.yaml", id, version)
	filePath := filepath.Join(dirPath, fileName)

	err = os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating directories for %s: %w", filePath, err)
	}

	if err := os.WriteFile(filePath, b.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing YAML file: %w", err)
	}

	return nil
}

func snakeCase(in string) string {
	return strings.TrimSpace(
		strings.ReplaceAll(
			strings.ReplaceAll(in, ".", "_"), "-", "_"))
}

func copyNonTemplateFile(templatePath, relativeFilepath, outputDir string, logger hclog.Logger) error {
	outputPath := filepath.Join(outputDir, relativeFilepath)
	if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for %s: %w", outputPath, err)
	}

	// Copy file contents
	srcFile, err := os.Open(templatePath)
	if err != nil {
		return fmt.Errorf("error opening source file %s: %w", templatePath, err)
	}
	defer func() {
		err := srcFile.Close()
		if err != nil {
			logger.Error("error closing output file %s: %w", templatePath, err)
		}
	}()

	dstFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating destination file %s: %w", outputPath, err)
	}
	defer func() {
		_ = dstFile.Close()
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("error copying file to %s: %w", outputPath, err)
	}

	// Try to preserve file mode from source
	if fi, err := os.Stat(templatePath); err == nil {
		_ = os.Chmod(outputPath, fi.Mode())
	}

	return nil
}
