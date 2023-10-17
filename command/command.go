package command

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// SetBase sets the base flags for all commands
func SetBase(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", defaultConfigPath(), "Configuration File, JSON or YAML")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().StringP("tactic", "t", "default", "Named set of strikes to execute from the raid")
	viper.BindPFlag("tactic", cmd.PersistentFlags().Lookup("tactic"))

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Louder now! Set log verbosity to INFO")
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	cmd.PersistentFlags().BoolP("silent", "s", false, "Shh! Only show essential log information")
	viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().BoolP("help", "h", false, "Give me a heading! Help for the specified command")
}

// InitializeConfig reads in config file and ENV variables if set.
func InitializeConfig() {

	viper.SetDefault("loglevel", "Error")
	viper.SetConfigFile(viper.GetString("config"))
	setLogLevelFromFlag()
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Print(err.Error())
	}

	// TODO: Logging at this location has unexpected behavior related to the WriteDirectory and loglevel
	// raidengine.GetLogger("overview", false).Debug("Lauching Privateer with loglevel '%s' and config file: %s", loglevel, viper.GetString("config"))
}

// setLogLevelFromFlag sets the log level based on the verbose and silent flags
func setLogLevelFromFlag() string {
	// If verbose is set, and loglevel is not trace (highest level), set loglevel to debug
	if viper.GetBool("verbose") && !strings.EqualFold(viper.GetString("loglevel"), "trace") {
		viper.Set("loglevel", "debug")
	} else if viper.GetBool("silent") {
		viper.Set("loglevel", "off")
	}
	return viper.GetString("loglevel")
}

// defaultConfigPath returns the default config path
func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
