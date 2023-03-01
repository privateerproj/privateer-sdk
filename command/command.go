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

	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	viper.SetDefault("loglevel", "info")
	loglevel := viper.GetString("loglevel")
	if viper.GetBool("verbose") {
		loglevel = "info"
	} else if viper.GetBool("silent") {
		loglevel = "off"
	}

	viper.Set("loglevel", loglevel)
	logger := logging.GetLogger("execution", loglevel, false)

	logger.Trace(fmt.Sprintf("Using config file: %s (loglevel: %s)", viper.GetString("config"), viper.GetString("loglevel")))

	if err := viper.ReadInConfig(); err != nil {
		logger.Debug(err.Error())
	}
}

func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
