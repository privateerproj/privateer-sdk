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
	_ = viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().StringP("loglevel", "l", "error", "Log level (trace, debug, info, warn, error, off)")
	_ = viper.BindPFlag("loglevel", cmd.PersistentFlags().Lookup("loglevel"))

	cmd.PersistentFlags().StringP("service", "s", "", "Named service to execute from the config")
	_ = viper.BindPFlag("service", cmd.PersistentFlags().Lookup("service"))

	cmd.PersistentFlags().StringP("test-suites", "t", "default", "Named set of test sets to execute from the plugin")
	_ = viper.BindPFlag("test-suites", cmd.PersistentFlags().Lookup("test-suites"))

	cmd.PersistentFlags().BoolP("silent", "", false, "Only show essential log information")
	_ = viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().BoolP("write", "", true, "Keep all of the detailed result outputs in a file. Disabling does not disable log files.")
	_ = viper.BindPFlag("write", cmd.PersistentFlags().Lookup("write"))

	cmd.PersistentFlags().BoolP("help", "h", false, "Give me a heading! Help for the specified command")
}

func ReadConfig() {
	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Print("[ERROR] " + err.Error())
	}
}

func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
