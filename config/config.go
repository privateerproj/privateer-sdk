package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/privateerproj/privateer-sdk/config/setter"
	"github.com/privateerproj/privateer-sdk/utils"
)

// GlobalConfig ...
var GlobalConfig GlobalOpts

func init() {
	GlobalConfig.Init() // Initialize with default values only
}

// Init ...
func (ctx *GlobalOpts) Init() {
	ctx.StartTime = time.Now()
	if ctx.VarsFile != "" {
		ctx.decode()
	}
	ctx.setEnvAndDefaults()
}

// decode uses an SDK helper to create a YAML file decoder,
// parse the file to an object, then extracts the values from
// Raids.Kubernetes into this context
func (ctx *GlobalOpts) decode() (err error) {
	configDecoder, file, err := NewConfigDecoder(ctx.VarsFile)
	if err != nil {
		return
	}
	err = configDecoder.Decode(&ctx)
	file.Close()
	return
}

// LogConfigState ...
func (ctx *GlobalOpts) LogConfigState() {
	json, _ := json.MarshalIndent(ctx, "", "  ")
	log.Printf("[INFO] Config State: %s", json)
}

// setEnvOrDefaults will set value from os.Getenv and default to the specified value
func (ctx *GlobalOpts) setEnvAndDefaults() {
	// Notes on SetVar's values:
	// 1. Pointer to local object; will be overwritten by env or default if empty
	// 2. Name of env var to check
	// 3. Default value to set if flags, vars file, and env have not provided a value

	home, _ := os.UserHomeDir()
	setter.SetVar(&ctx.InstallDir, "PVTR_INSTALL_DIR", filepath.Join(home, "privateer"))

	setter.SetVar(&ctx.TmpDir, "PVTR_TMP_DIR", filepath.Join(ctx.InstallDir, "tmp"))
	setter.SetVar(&ctx.WriteDirectory, "PVTR_WRITE_DIRECTORY", ctx.outputDir())
	setter.SetVar(&ctx.LogLevel, "PVTR_LOG_LEVEL", "DEBUG")
	setter.SetVar(&ctx.GodogResultsFormat, "PVTR_RESULTS_FORMAT", "cucumber")
}

// ParseTags takes two lists of tags and parses them into a cucumber tag string
// Tags may start with '@' or '~@' respectively, but it is not required
func ParseTags(inclusions, exclusions []string) string {
	var tags []string
	if len(inclusions) > 0 {
		tags = append(tags, parseInclusions(inclusions))
	}
	if len(exclusions) > 0 {
		tags = append(tags, parseExclusions(exclusions))
	}
	// If only one is provided, this joiner won't be used
	return strings.Join(tags, " && ")
}

func parseInclusions(inclusions []string) string {
	inclusions = prependTags(inclusions, "@")
	return strings.Join(inclusions, ",")
}

func parseExclusions(exclusions []string) string {
	exclusions = prependTags(exclusions, "~@")
	return strings.Join(exclusions, " && ")
}

func prependTags(tags []string, prefix string) []string {
	for i, value := range tags {
		// ensure value is not empty, then force it to begin with prefix
		// prefix value is expected to not be found anywhere but as the prefix
		if len(value) > 0 && !strings.Contains(value, prefix) {
			tags[i] = prefix + value
		}
	}
	return tags
}

// CleanupTmp is used to dispose of any temp resources used during execution
func (ctx *GlobalOpts) CleanupTmp() {
	err := os.RemoveAll(ctx.TmpDir)
	if err != nil {
		log.Printf("[ERROR] Failed to remove temporary directory %v", err)
	}
}

// outputDir parses a filepath based on GlobalOpts.InstallDir and the datetime this was initialized
func (ctx *GlobalOpts) outputDir() string {
	execName := utils.GetExecutableName()
	if execName == "privateer" {
		return filepath.Join(ctx.InstallDir, "output")
	}
	return filepath.Join(ctx.InstallDir, "output", execName)
}

// PrepareOutputDirectory will ensure readiness of output dir and specified subdirectories
func (ctx *GlobalOpts) PrepareOutputDirectory(subdirectories ...string) {
	log.Printf("[DEBUG] Ensuring output directory is ready for use: %s", ctx.WriteDirectory)
	ensureDirReadiness(ctx.WriteDirectory)
	// If subdirectories are provided, validate each
	for _, dir := range subdirectories {
		dirName := filepath.Join(ctx.WriteDirectory, dir)
		ensureDirReadiness(dirName)
	}
}

// TODO: fix that this is defaulting to creating a new directory when using -h
// Create directory if possible, otherwise log error to console
func ensureDirReadiness(directory string) {
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		log.Print(utils.ReformatError(err.Error()))
	} else {
		log.Printf("[DEBUG] Directory is ready for use: %s", directory)
	}
}
