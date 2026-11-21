package harness

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/oci"
)

func TestSetHarnessFlags(t *testing.T) {
	cmd := &cobra.Command{}
	SetHarnessFlags(cmd)

	for _, name := range []string{"hub-url", "autoinstall", "require-signed-plugins"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected --%s flag to be registered", name)
		}
	}
	if got := cmd.PersistentFlags().Lookup("hub-url").DefValue; got != oci.DefaultHubURL {
		t.Errorf("--hub-url default = %q, want %q", got, oci.DefaultHubURL)
	}
	if got := cmd.PersistentFlags().Lookup("autoinstall").DefValue; got != "false" {
		t.Errorf("--autoinstall default = %q, want false", got)
	}
}

func TestRequireSignedPluginsFlag(t *testing.T) {
	t.Cleanup(viper.Reset)
	cmd := &cobra.Command{}
	SetHarnessFlags(cmd)

	if got := cmd.PersistentFlags().Lookup("require-signed-plugins").DefValue; got != "false" {
		t.Errorf("--require-signed-plugins default = %q, want false", got)
	}
	if err := cmd.PersistentFlags().Parse([]string{"--require-signed-plugins"}); err != nil {
		t.Fatal(err)
	}
	if !config.RequireSignedPlugins() {
		t.Error("RequireSignedPlugins() = false after --require-signed-plugins, want true")
	}
}
