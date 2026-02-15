package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func resetViper() {
	viper.Reset()
}

// resolvedPath returns the symlink-resolved absolute path.
// On macOS, /var is a symlink to /private/var, so viper may
// return a resolved path that differs from os.MkdirTemp output.
func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("failed to resolve path %q: %v", path, err)
	}
	return resolved
}

func TestSetBase_ConfigFlagDefaultIsEmpty(t *testing.T) {
	resetViper()
	cmd := &cobra.Command{Use: "test"}
	SetBase(cmd)

	flag := cmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("expected config flag to be registered")
	}
	if flag.DefValue != "" {
		t.Errorf("expected config flag default to be empty, got %q", flag.DefValue)
	}
}

func TestReadConfig_ExplicitConfigPath(t *testing.T) {
	resetViper()

	tmpDir, err := os.MkdirTemp("", "privateer-sdk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configFile := filepath.Join(tmpDir, "custom.yml")
	if err := os.WriteFile(configFile, []byte("loglevel: debug\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	viper.Set("config", configFile)
	ReadConfig()

	if resolvedPath(t, viper.ConfigFileUsed()) != resolvedPath(t, configFile) {
		t.Errorf("expected config file %q, got %q", resolvedPath(t, configFile), resolvedPath(t, viper.ConfigFileUsed()))
	}
	if viper.GetString("loglevel") != "debug" {
		t.Errorf("expected loglevel 'debug', got %q", viper.GetString("loglevel"))
	}
}

func TestReadConfig_SearchesCwd(t *testing.T) {
	resetViper()

	tmpDir, err := os.MkdirTemp("", "privateer-sdk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte("loglevel: trace\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	ReadConfig()

	if resolvedPath(t, viper.ConfigFileUsed()) != resolvedPath(t, configFile) {
		t.Errorf("expected config file %q, got %q", resolvedPath(t, configFile), resolvedPath(t, viper.ConfigFileUsed()))
	}
	if viper.GetString("loglevel") != "trace" {
		t.Errorf("expected loglevel 'trace', got %q", viper.GetString("loglevel"))
	}
}

func TestReadConfig_SearchesHomeDotPrivateer(t *testing.T) {
	resetViper()

	tmpDir, err := os.MkdirTemp("", "privateer-sdk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create ~/.privateer equivalent with config
	privateerDir := filepath.Join(tmpDir, ".privateer")
	if err := os.MkdirAll(privateerDir, 0755); err != nil {
		t.Fatalf("failed to create .privateer dir: %v", err)
	}
	configFile := filepath.Join(privateerDir, "config.yml")
	if err := os.WriteFile(configFile, []byte("loglevel: info\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Override HOME so UserHomeDir returns our tmpDir
	origHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", origHome) }()
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	// Chdir to a dir with no config.yml so cwd search misses
	emptyDir, err := os.MkdirTemp("", "privateer-sdk-empty-*")
	if err != nil {
		t.Fatalf("failed to create empty dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(emptyDir) }()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(emptyDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	ReadConfig()

	if resolvedPath(t, viper.ConfigFileUsed()) != resolvedPath(t, configFile) {
		t.Errorf("expected config file %q, got %q", resolvedPath(t, configFile), resolvedPath(t, viper.ConfigFileUsed()))
	}
	if viper.GetString("loglevel") != "info" {
		t.Errorf("expected loglevel 'info', got %q", viper.GetString("loglevel"))
	}
}

func TestReadConfig_CwdTakesPrecedenceOverHome(t *testing.T) {
	resetViper()

	tmpDir, err := os.MkdirTemp("", "privateer-sdk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create ~/.privateer config
	privateerDir := filepath.Join(tmpDir, ".privateer")
	if err := os.MkdirAll(privateerDir, 0755); err != nil {
		t.Fatalf("failed to create .privateer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(privateerDir, "config.yml"), []byte("loglevel: warn\n"), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// Create cwd config
	cwdDir, err := os.MkdirTemp("", "privateer-sdk-cwd-*")
	if err != nil {
		t.Fatalf("failed to create cwd dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(cwdDir) }()

	cwdConfig := filepath.Join(cwdDir, "config.yml")
	if err := os.WriteFile(cwdConfig, []byte("loglevel: debug\n"), 0644); err != nil {
		t.Fatalf("failed to write cwd config: %v", err)
	}

	// Override HOME
	origHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", origHome) }()
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	// Chdir to cwd with config
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(cwdDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	ReadConfig()

	if resolvedPath(t, viper.ConfigFileUsed()) != resolvedPath(t, cwdConfig) {
		t.Errorf("expected cwd config %q to take precedence, got %q", resolvedPath(t, cwdConfig), resolvedPath(t, viper.ConfigFileUsed()))
	}
	if viper.GetString("loglevel") != "debug" {
		t.Errorf("expected loglevel 'debug' from cwd config, got %q", viper.GetString("loglevel"))
	}
}
