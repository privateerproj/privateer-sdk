package command

// Wontfix: Logging in this file has unexpected behavior related to the WriteDirectory and loglevel values.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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

}

// InitializeConfig reads in config file and ENV variables if set.
func InitializeConfig() {

	viper.SetDefault("loglevel", "Error")
	viper.SetDefault("WriteDirectory", defaultWritePath())
	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Print(err.Error())
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

func defaultWritePath() string {
	home, err := os.UserHomeDir()
	datetime := time.Now().Local().Format(time.RFC3339)
	dirName := strings.Replace(datetime, ":", "", -1)
	if err != nil {
		return ""
	}
	return filepath.Join(home, "privateer", "logs", dirName)
}
