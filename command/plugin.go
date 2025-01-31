package command

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/pluginkit"
	"github.com/privateerproj/privateer-sdk/shared"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Plugin struct{}

var ActiveVessel pluginkit.Vessel

// Start will be called by Privateer via gRPC
func (p *Plugin) Start() (err error) {
	err = ActiveVessel.Mobilize()
	return
}

func NewPluginCommands(
	pluginName, buildVersion, buildGitCommitHash, buildTime string,
	armory *pluginkit.Armory,
	initializer func(*config.Config) error,
	requiredVars []string) *cobra.Command {

	ActiveVessel = pluginkit.NewVessel(
		pluginName,
		armory,
		initializer,
		requiredVars,
	)

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
			err := ActiveVessel.Mobilize()
			if err != nil {
				log.Fatal(err)
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
				fmt.Fprintf(writer, "Version:\t%s\n", buildVersion)
				fmt.Fprintf(writer, "Commit:\t%s\n", buildGitCommitHash)
				fmt.Fprintf(writer, "Build Time:\t%s\n", buildTime)
				writer.Flush()
			} else {
				fmt.Println(buildVersion)
			}
		},
	}
}
