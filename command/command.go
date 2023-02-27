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

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Louder now! Increase log verbosity to maximum")
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	cmd.PersistentFlags().BoolP("silent", "s", false, "Shh! Only show essential log information")
	viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().BoolP("help", "h", false, fmt.Sprintf("Give me a heading! Help for the specified command"))
	fmt.Print("1\n")
}

func InitializeConfig() {
	logger := logging.GetLogger("setup", "error", false)

	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	viper.SetDefault("loglevel", "info")
	loglevel := viper.GetString("loglevel")
	if viper.GetBool("verbose") {
		loglevel = "trace"
	} else if viper.GetBool("silent") {
		loglevel = "off"
	}
	logger.Trace(fmt.Sprintf("Loglevel: %v", loglevel))

	viper.Set("loglevel", loglevel)
	logger = logging.GetLogger("execution", loglevel, false)

	logger.Trace(fmt.Sprintf("Config file flag: %s (loglevel: %s)", viper.GetString("config"), viper.GetString("loglevel")))

	if err := viper.ReadInConfig(); err != nil {
		logger.Error(err.Error())
		fmt.Print("???")
	}
	msg := fmt.Sprintf("Using config file: %s (loglevel: %s)", viper.ConfigFileUsed(), viper.GetString("loglevel"))
	logger.Trace(msg) // TODO: this doesn't print within the raid even with loglevel set
}

func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}
