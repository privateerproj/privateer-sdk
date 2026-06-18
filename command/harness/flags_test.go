package harness

import (
	"testing"

	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/spf13/cobra"
)

func TestSetHarnessFlags(t *testing.T) {
	cmd := &cobra.Command{}
	SetHarnessFlags(cmd)

	for _, name := range []string{"hub-url", "autoinstall"} {
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
