package command

// Wontfix: Logging in this file has unexpected behavior related to the WriteDirectory and loglevel values.

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// SetBase sets the base flags for all commands
func SetBase(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", defaultConfigPath(), "Configuration File, JSON or YAML")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().StringP("loglevel", "l", "error", "Log level (trace, debug, info, warn, error, off)")
	viper.BindPFlag("loglevel", cmd.PersistentFlags().Lookup("loglevel"))

	cmd.PersistentFlags().StringP("service", "s", "", "Named service to execute from the config")
	viper.BindPFlag("service", cmd.PersistentFlags().Lookup("service"))

	cmd.PersistentFlags().StringP("tactic", "t", "default", "Named set of strikes to execute from the raid")
	viper.BindPFlag("tactic", cmd.PersistentFlags().Lookup("tactic"))

	cmd.PersistentFlags().BoolP("silent", "", false, "Shh! Only show essential log information")
	viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().BoolP("help", "h", false, "Give me a heading! Help for the specified command")

	// Initialize Viper
	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Print("[ERROR] " + err.Error())
	}
}

// defaultConfigPath returns the default config path
func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
