package command

import (
	"testing"

	"github.com/privateerproj/privateer-sdk/pluginkit"
)

var (
	pluginName         = "test"
	buildVersion       = "1.0.0"
	buildGitCommitHash = "123"
	buildTime          = "2020-01-01T00:00:00Z"
)

func TestNewPluginCommands(t *testing.T) {
	payload := interface{}(nil)
	requiredVars := []string{}
	vessel := pluginkit.NewVessel(pluginName, payload, requiredVars)

	cmd := NewPluginCommands(pluginName, buildVersion, buildGitCommitHash, buildTime, vessel)
	if cmd.Use != pluginName {
		t.Errorf("Expected cmd.Use to be %s, but got %s", pluginName, cmd.Use)
	}
	if cmd.Short != "Test suite for test." {
		t.Errorf("Expected cmd.Short to be 'Test suite for test.', but got %s", cmd.Short)
	}
	if cmd.PersistentPreRun == nil {
		t.Error("Expected cmd.PersistentPreRun to be set")
	}
	if cmd.Run == nil {
		t.Error("Expected cmd.Run to be set")
	}
}

func TestRunCommand(t *testing.T) {
	runCmd := runCommand(pluginName)
	if runCmd.Use != pluginName {
		t.Errorf("Expected runCmd.Use to be %s, but got %s", pluginName, runCmd.Use)
	}
	if runCmd.Short != "Test suite for test." {
		t.Errorf("Expected runCmd.Short to be 'Test suite for test.', but got %s", runCmd.Short)
	}
	if runCmd.PersistentPreRun == nil {
		t.Error("Expected runCmd.PersistentPreRun to be set")
	}
	if runCmd.Run == nil {
		t.Error("Expected runCmd.Run to be set")
	}
}

func TestDebugCommand(t *testing.T) {
	cmd := debugCommand()
	if cmd.Use != "debug" {
		t.Errorf("Expected cmd.Use to be 'debug', but got %s", cmd.Use)
	}
	if cmd.Short != "Run the Plugin in debug mode" {
		t.Errorf("Expected cmd.Short to be 'Run the Plugin in debug mode', but got %s", cmd.Short)
	}
	if cmd.Run == nil {
		t.Error("Expected cmd.Run to be set")
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := versionCommand(buildVersion, buildGitCommitHash, buildTime)
	if cmd.Use != "version" {
		t.Errorf("Expected cmd.Use to be 'version', but got %s", cmd.Use)
	}
	if cmd.Short != "Display version details." {
		t.Errorf("Expected cmd.Short to be 'Display version details.', but got %s", cmd.Short)
	}
	if cmd.Run == nil {
		t.Error("Expected cmd.Run to be set")
	}
}
