package command

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/shared"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Plugin represents a Privateer plugin instance.
type Plugin struct{}

// ActiveEvaluationOrchestrator is the currently active evaluation orchestrator.
var ActiveEvaluationOrchestrator *pluginkit.EvaluationOrchestrator

// Start will be called by Privateer via gRPC.
func (p *Plugin) Start() error {
	return ActiveEvaluationOrchestrator.Mobilize()
}

// NewPluginCommands creates a new cobra command for the plugin with version and orchestrator support.
func NewPluginCommands(pluginName, buildVersion, buildGitCommitHash, buildTime string, orchestrator *pluginkit.EvaluationOrchestrator) *cobra.Command {

	ActiveEvaluationOrchestrator = orchestrator

	runCmd := runCommand(pluginName)

	runCmd.AddCommand(debugCommand())

	runCmd.AddCommand(
		versionCommand(buildVersion, buildGitCommitHash, buildTime))

	SetBase(runCmd)
	return runCmd
}

func runCommand(pluginName string) *cobra.Command {
	return &cobra.Command{
		Use:   pluginName,
		Short: fmt.Sprintf("Test suite for %s.", pluginName),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ReadConfig()
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Serve plugin
			plugin := &Plugin{}
			serveOpts := &shared.ServeOpts{
				Plugin: plugin,
			}

			shared.Serve(pluginName, serveOpts)
		},
	}
}

func debugCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "debug",
		Short: "Run the Plugin in debug mode",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Print("Running in debug mode\n")
			err := ActiveEvaluationOrchestrator.Mobilize()
			if err != nil {
				cmd.Println(err)
			}
		},
	}
}

func versionCommand(
	buildVersion, buildGitCommitHash, buildTime string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version details.",
		Run: func(cmd *cobra.Command, args []string) {
			writer := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
			if viper.GetBool("verbose") {
				_, _ = fmt.Fprintf(writer, "Version:\t%s\n", buildVersion)
				_, _ = fmt.Fprintf(writer, "Commit:\t%s\n", buildGitCommitHash)
				_, _ = fmt.Fprintf(writer, "Build Time:\t%s\n", buildTime)
				_ = writer.Flush()
			} else {
				fmt.Println(buildVersion)
			}
		},
	}
}
