package harness

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/internal/oci"
)

// SetHarnessFlags registers the harness-global flags on a harness's root command
// (the pvtr CLI calls this on its root, alongside command.SetBase).
//
// These are HARNESS concerns — they drive install/publish/run, not a plugin
// serving itself — so they live here rather than in command.SetBase. Keeping
// them out of SetBase also keeps the oci dependency out of the plugin-facing
// surface (plugins import package command, never command/harness).
//
// Each flag binds to the same viper key its config.yml / PVTR_* env equivalent
// uses, so a value may come from flag, env, or config file. Precedence (highest
// first): flag > PVTR_* env > config.yml > the default shown here.
func SetHarnessFlags(cmd *cobra.Command) {
	// hub-url: the single configured grc.store endpoint; the OCI registry host is
	// discovered from it (see oci.HubURL / oci.Discovery). Also settable via the
	// hub-url config.yml key or the PVTR_HUB_URL environment variable.
	cmd.PersistentFlags().String("hub-url", oci.DefaultHubURL, "grc.store hub base URL; the registry host is discovered from it")
	_ = viper.BindPFlag("hub-url", cmd.PersistentFlags().Lookup("hub-url"))

	// autoinstall: when true, a `pvtr run` first installs any config-requested
	// plugins that are not yet installed (see config.AutoInstall / the run
	// preflight). Also settable via the autoinstall config.yml key or the
	// PVTR_AUTOINSTALL environment variable.
	cmd.PersistentFlags().Bool("autoinstall", false, "Before a run, install any config-requested plugins that are not yet installed")
	_ = viper.BindPFlag("autoinstall", cmd.PersistentFlags().Lookup("autoinstall"))
}
