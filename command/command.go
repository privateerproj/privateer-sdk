package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/logging"
)

func SetBase(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", defaultConfigPath(), "Configuration File, JSON or YAML")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Louder now! Set log verbosity to INFO")
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	cmd.PersistentFlags().BoolP("silent", "s", false, "Shh! Only show essential log information")
	viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().BoolP("help", "h", false, "Give me a heading! Help for the specified command")
}

func InitializeConfig() {

	loglevel := setLogLevelFromFlag()
	viper.Set("loglevel", loglevel)

	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	logger := logging.GetLogger("execution", loglevel, false)

	logger.Debug(fmt.Sprintf("Reading config file: %s (loglevel: %s)", viper.GetString("config"), loglevel))
	if err := viper.ReadInConfig(); err != nil {
		logger.Debug(err.Error())
	}
}

func setLogLevelFromFlag() string {
	viper.SetDefault("loglevel", "error")
	if viper.GetBool("verbose") {
		return "debug"
	} else if viper.GetBool("silent") {
		return "off"
	}
	return viper.GetString("loglevel")
}

func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
