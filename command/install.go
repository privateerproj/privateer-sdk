package command

import (
	"fmt"

	"github.com/privateerproj/privateer-sdk/internal/install"
	"github.com/spf13/cobra"
)

// GetInstallCmd returns the install command that can be added to a root command.
// writerFn is called at command execution time, so the writer can be
// initialized lazily (e.g. in a PersistentPreRun hook). The install logic lives
// in internal/install; this is the CLI seam that owns the writer and dispatches
// between the grc.store and --local paths.
//
// Deprecated: use harness.GetInstallCmd instead. This will be removed once the
// pvtr CLI migrates to the command/harness import path.
func GetInstallCmd(writerFn func() Writer) *cobra.Command {
	var localPath string
	var fromConfig bool

	installCmd := &cobra.Command{
		Use:   "install [<namespace>/<plugin_id>[@<version>]]",
		Short: "Install a verified plugin from grc.store, or a local path.",
		Long: "Install a plugin from grc.store by its <namespace>/<plugin_id> coordinate " +
			"(optionally pinned with @<version>; defaults to the latest). The signed OCI " +
			"index is pulled and verified end-to-end (signature + signer identity + digest " +
			"chain) before anything is written. Use --local to install a local plugin binary, " +
			"or --from-config to install every plugin the active config references that is " +
			"not yet installed.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := writerFn()
			defer func() { _ = w.Flush() }()
			if localPath != "" {
				return install.Local(w, localPath)
			}
			if fromConfig {
				return install.FromConfig(cmd.Context(), w)
			}
			if len(args) == 0 {
				return fmt.Errorf("a plugin coordinate <namespace>/<plugin_id> is required (or use --local or --from-config)")
			}
			return install.FromStore(cmd.Context(), w, args[0])
		},
	}
	installCmd.Flags().StringVar(&localPath, "local", "", "Path to a local plugin binary to install")
	installCmd.Flags().BoolVar(&fromConfig, "from-config", false, "Install all not-yet-installed plugins referenced by the active config (concurrently)")
	return installCmd
}
