package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/logging"
)

// SetBase sets the base flags for all commands
func SetBase(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", defaultConfigPath(), "Configuration File, JSON or YAML")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

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
	loglevel := setLogLevelFromFlag()
	viper.AutomaticEnv()
	logger := logging.GetLogger("execution", loglevel, false)

	logger.Info(fmt.Sprintf("Reading config file: %s (loglevel: %s)", viper.GetString("config"), loglevel))
	if err := viper.ReadInConfig(); err != nil {
		logger.Debug(err.Error())
	}
}

// setLogLevelFromFlag sets the log level based on the verbose and silent flags
func setLogLevelFromFlag() string {
	if viper.GetBool("verbose") {
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
