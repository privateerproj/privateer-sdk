package command

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/logging"
)

func SetBase(cmd *cobra.Command) {
	cobra.OnInitialize(initConfig)
	cmd.PersistentFlags().StringP("config", "c", defaultConfigPath(), "Configuration File, JSON or YAML")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Louder now! Increase log verbosity to maximum.")
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))

	cmd.PersistentFlags().BoolP("silent", "s", false, "Shh! Only show essential log information.")
	viper.BindPFlag("silent", cmd.PersistentFlags().Lookup("silent"))

	cmd.PersistentFlags().StringP("binaries-path", "b", defaultBinariesPath(), "The Armory! Path to the location where raid binaries are stored.")
	viper.BindPFlag("binaries-path", cmd.PersistentFlags().Lookup("binaries-path"))
}

func initConfig() {
	logger := logging.UseLogger("core", "error")

	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	viper.SetDefault("loglevel", "info")
	loglevel := viper.GetString("loglevel")
	if viper.GetBool("verbose") {
		loglevel = "trace"
	} else if viper.GetBool("silent") {
		loglevel = "off"
	}
	viper.Set("loglevel", loglevel)
	logger = logging.UseLogger("core", loglevel)

	if err := viper.ReadInConfig(); err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			logger.Debug(err.Error())
		} else {
			logger.Error(err.Error())
			os.Exit(1)
		}
	} else {
		msg := fmt.Sprintf("Using config file: %s (loglevel: %s)", viper.ConfigFileUsed(), viper.GetString("loglevel"))
		logger.Trace(msg)
	}
}

func defaultConfigPath() string {
	workDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(workDir, "config.yml")
}

func defaultBinariesPath() string {
	home, _ := os.UserHomeDir() // sue me
	return path.Join(home, "privateer", "bin")
}
