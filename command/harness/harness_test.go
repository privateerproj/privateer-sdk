package harness

import (
	"testing"

	"github.com/privateerproj/privateer-sdk/command"
)

// writerFn is a trivial Writer factory for constructing commands in tests; the
// constructors only invoke it at execution time, so returning nil here is fine.
func writerFn() Writer { return nil }

// TestConstructorsForward checks each forwarded command constructor returns a
// usable cobra command, i.e. the alias surface is wired to command/.
func TestConstructorsForward(t *testing.T) {
	if GetInstallCmd(writerFn) == nil {
		t.Error("GetInstallCmd returned nil")
	}
	if GetListCmd(writerFn) == nil {
		t.Error("GetListCmd returned nil")
	}
	if GetPublishCmd(writerFn) == nil {
		t.Error("GetPublishCmd returned nil")
	}
	if GetLoginCmd(writerFn) == nil {
		t.Error("GetLoginCmd returned nil")
	}
	if GetLogoutCmd(writerFn) == nil {
		t.Error("GetLogoutCmd returned nil")
	}
}

// TestTypeAliasIdentity confirms the aliases are identity-preserving: a
// *command.PluginPkg and a *harness.PluginPkg are the same type, so values
// returned by command flow through the harness surface without conversion.
func TestTypeAliasIdentity(t *testing.T) {
	var hp *PluginPkg = command.NewPluginPkg("ossf/x", "", "svc") // command -> harness type
	var cp *command.PluginPkg = hp                                // harness  -> command type
	if cp == nil {
		t.Fatal("expected a non-nil PluginPkg")
	}
	// Contains takes []*harness.PluginPkg; feed it the command-built value.
	if Contains([]*PluginPkg{hp}, "ossf/x") != true {
		t.Error("Contains did not find the plugin by name")
	}
}
