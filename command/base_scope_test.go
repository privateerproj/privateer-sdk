package command

import (
	"testing"

	"github.com/spf13/cobra"
)

// universalFlags are the flags that apply to every command and therefore
// belong on the shared root via SetBase.
var universalFlags = []string{"config", "loglevel"}

// runScopedFlags are execution-specific flags that only make sense for a run.
// They must not leak onto the shared root, where their shorthands can collide
// with sibling subcommands (e.g. -o vs generate-plugin's --output-dir).
var runScopedFlags = map[string]string{
	"output":          "o",
	"write-directory": "w",
	"service":         "s",
	"test-suites":     "t",
	"silent":          "",
	"write":           "",
	"include-payload": "",
}

// TestSetBase_RegistersUniversalFlags asserts SetBase keeps the truly universal
// flags on the command it is given.
func TestSetBase_RegistersUniversalFlags(t *testing.T) {
	resetViper()
	cmd := &cobra.Command{Use: "test"}
	SetBase(cmd)

	for _, name := range universalFlags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected universal flag %q to be registered by SetBase", name)
		}
	}
}

// TestSetBase_DoesNotRegisterRunScopedFlags asserts SetBase no longer leaks the
// run-specific flags onto the command. Leaking -o (output) onto a shared root
// is what caused the cobra shorthand-collision panic in generate-plugin.
func TestSetBase_DoesNotRegisterRunScopedFlags(t *testing.T) {
	resetViper()
	cmd := &cobra.Command{Use: "test"}
	SetBase(cmd)

	for name := range runScopedFlags {
		if cmd.PersistentFlags().Lookup(name) != nil {
			t.Errorf("run-specific flag %q should not be registered by SetBase", name)
		}
	}
}

// TestSetRunFlags_RegistersRunScopedFlags asserts the run-specific flags, with
// their expected shorthands, live on the run command instead.
func TestSetRunFlags_RegistersRunScopedFlags(t *testing.T) {
	resetViper()
	cmd := &cobra.Command{Use: "run"}
	SetRunFlags(cmd)

	for name, short := range runScopedFlags {
		flag := cmd.PersistentFlags().Lookup(name)
		if flag == nil {
			t.Errorf("expected run flag %q to be registered by SetRunFlags", name)
			continue
		}
		if flag.Shorthand != short {
			t.Errorf("flag %q: expected shorthand %q, got %q", name, short, flag.Shorthand)
		}
	}
}

// TestSetBase_NoShorthandCollisionWithSubcommand is the regression guard for the
// generate-plugin panic. When SetBase is applied to a shared root and a
// subcommand claims -o for its own flag, cobra merges the parent's persistent
// flags at Execute time; a duplicate -o shorthand would panic. With output
// de-scoped off SetBase, the two coexist.
func TestSetBase_NoShorthandCollisionWithSubcommand(t *testing.T) {
	resetViper()
	root := &cobra.Command{Use: "pvtr"}
	SetBase(root)

	sub := &cobra.Command{Use: "generate-plugin", Run: func(*cobra.Command, []string) {}}
	sub.Flags().StringP("output-dir", "o", "generated-plugin/", "")
	root.AddCommand(sub)
	root.SetArgs([]string{"generate-plugin"})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic from -o shorthand collision: %v", r)
		}
	}()
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error executing subcommand: %v", err)
	}
}
