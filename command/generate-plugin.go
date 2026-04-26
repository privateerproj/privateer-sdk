package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v5"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/gemaraproj/go-gemara"
	"github.com/gemaraproj/go-gemara/fetcher"
	"github.com/privateerproj/privateer-sdk/utils"
)

// PluginConfig holds the validated configuration for plugin generation.
type PluginConfig struct {
	TemplatesDir string
	SourcePath   string
	OutputDir    string
	ServiceName  string
	Organization string
}

// CatalogData extends gemara.ControlCatalog with additional fields for plugin generation.
type CatalogData struct {
	gemara.ControlCatalog
	ServiceName             string
	Organization            string
	Requirements            []Req
	ApplicabilityCategories []string
	StrippedName            string
}

// Req represents an assessment requirement with an ID and text description.
type Req struct {
	Id   string
	Text string
}

// GeneratePlugin executes the plugin generation flow and returns an
// exit code from the same set used by Run (TestPass, TestFail, InternalError,
// BadUsage). Mirrors Run's shape: classification + logging happen here so the
// CLI just calls os.Exit(GeneratePlugin(logger)).
func GeneratePlugin(logger hclog.Logger) (exitCode int) {
	cfg, exitCode := setupTemplatingEnvironment(logger)
	if exitCode != TestPass {
		return exitCode
	}
	return generatePlugin(logger, cfg)
}

// generatePlugin generates a plugin from a catalog file. Returns an exit code
// from the run.go set: TestPass on success, TestFail if some templates failed
// to render but the rest succeeded, InternalError for I/O / fetch / parse
// failures.
func generatePlugin(logger hclog.Logger, cfg PluginConfig) (exitCode int) {
	data := CatalogData{}
	data.ServiceName = cfg.ServiceName
	data.Organization = cfg.Organization

	sourcePath, err := resolveSourcePath(cfg.SourcePath)
	if err != nil {
		logger.Error(fmt.Sprintf("invalid source path: %s", err))
		return InternalError
	}

	if err := data.LoadFiles(context.Background(), &fetcher.URI{}, []string{sourcePath}); err != nil {
		logger.Error(err.Error())
		return InternalError
	}

	if err := data.getAssessmentRequirements(); err != nil {
		logger.Error(err.Error())
		return InternalError
	}

	var renderFailures int
	err = filepath.Walk(cfg.TemplatesDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if genErr := generateFileFromTemplate(data, path, cfg.TemplatesDir, cfg.OutputDir, logger); genErr != nil {
				logger.Error(fmt.Sprintf("Failed while writing in dir '%s': %s", cfg.OutputDir, genErr))
				renderFailures++
			}
			return nil
		},
	)
	if err != nil {
		logger.Error(fmt.Sprintf("error walking through templates directory: %s", err))
		return InternalError
	}

	if err := writeCatalogFile(&data.ControlCatalog, cfg.OutputDir); err != nil {
		logger.Error(fmt.Sprintf("failed to write catalog to file: %s", err))
		return InternalError
	}

	if renderFailures > 0 {
		logger.Error(fmt.Sprintf("%d template(s) failed to render", renderFailures))
		return TestFail
	}

	return TestPass
}

// setupTemplatingEnvironment validates and sets up the environment for plugin
// generation. Returns the same exit codes as GeneratePlugin: BadUsage when
// a required flag is missing, InternalError for I/O failures, TestPass when
// the config is ready.
func setupTemplatingEnvironment(logger hclog.Logger) (PluginConfig, int) {
	cfg := PluginConfig{}

	cfg.SourcePath = viper.GetString("source-path")
	if cfg.SourcePath == "" {
		logger.Error("required: --source-path is required to generate a plugin from a control set from local file or URL")
		return cfg, BadUsage
	}

	cfg.ServiceName = viper.GetString("service-name")
	if cfg.ServiceName == "" {
		logger.Error("required: --service-name is required to generate a plugin")
		return cfg, BadUsage
	}

	cfg.Organization = viper.GetString("organization")
	if cfg.Organization == "" {
		logger.Error("required: --organization is required to generate a plugin")
		return cfg, BadUsage
	}

	if viper.GetString("local-templates") != "" {
		cfg.TemplatesDir = viper.GetString("local-templates")
	} else {
		cfg.TemplatesDir = filepath.Join(os.TempDir(), "privateer-templates")
		if err := setupTemplatesDir(cfg.TemplatesDir, logger); err != nil {
			logger.Error(fmt.Sprintf("error setting up templates directory: %s", err))
			return cfg, InternalError
		}
	}

	cfg.OutputDir = viper.GetString("output-dir")
	logger.Trace(fmt.Sprintf("Generated plugin will be stored in this directory: %s", cfg.OutputDir))

	if err := os.MkdirAll(cfg.OutputDir, utils.DirPermissions); err != nil {
		logger.Error(err.Error())
		return cfg, InternalError
	}

	return cfg, TestPass
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

	// If the file is not a template, copy it over with placeholder replacement
	if filepath.Ext(templatePath) != ".tmpl" {
		return copyNonTemplateFile(data, templatePath, relativeFilepath, outputDir)
	}

	tmpl, err := template.New("plugin").Funcs(template.FuncMap{
		"as_text": func(in string) string {
			return strings.TrimSpace(
				strings.ReplaceAll(in, "\n", " "))
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

	outputPath := filepath.Join(outputDir, strings.TrimSuffix(relativeFilepath, ".tmpl"))

	err = os.MkdirAll(filepath.Dir(outputPath), utils.DirPermissions)
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

	err = os.MkdirAll(dirPath, utils.DirPermissions)
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

// resolveSourcePath ensures the source path has a URI scheme.
// Bare file paths get resolved to absolute and prefixed with file://;
// all other schemes are passed through for the fetcher to validate.
//
// Absolute resolution matters because url.Parse("file://foo/bar") treats
// "foo" as the host and "/bar" as the path, so a relative input would silently
// reach the fetcher with the leading directory stripped.
func resolveSourcePath(sourcePath string) (string, error) {
	parsed, err := url.Parse(sourcePath)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		abs, err := filepath.Abs(sourcePath)
		if err != nil {
			return "", err
		}
		return "file://" + abs, nil
	}
	return sourcePath, nil
}

func copyNonTemplateFile(data CatalogData, templatePath, relativeFilepath, outputDir string) error {
	outputPath := filepath.Join(outputDir, relativeFilepath)
	if err := os.MkdirAll(filepath.Dir(outputPath), utils.DirPermissions); err != nil {
		return fmt.Errorf("error creating directories for %s: %w", outputPath, err)
	}

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("error reading source file %s: %w", templatePath, err)
	}

	// Replace placeholders in non-template files
	output := strings.ReplaceAll(string(content), "__SERVICE_NAME__", data.ServiceName)
	output = strings.ReplaceAll(output, "__ORGANIZATION__", data.Organization)

	// Try to preserve file mode from source
	mode := os.FileMode(0644)
	if fi, err := os.Stat(templatePath); err == nil {
		mode = fi.Mode()
	}

	if err := os.WriteFile(outputPath, []byte(output), mode); err != nil {
		return fmt.Errorf("error writing file %s: %w", outputPath, err)
	}

	return nil
}
