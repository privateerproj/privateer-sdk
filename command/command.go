package command

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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
}

func InitializeConfig(cmd *cobra.Command, args []string) {
	logger := logging.GetLogger("setup", "error", false)

	viper.SetConfigFile(viper.GetString("config"))
	viper.AutomaticEnv()

	viper.SetDefault("loglevel", "info")
	loglevel := viper.GetString("loglevel")
	log.Printf("Loglevel: %s", loglevel)
	if viper.GetBool("verbose") {
		loglevel = "trace"
	} else if viper.GetBool("silent") {
		loglevel = "off"
	}
	viper.Set("loglevel", loglevel)
	logger = logging.GetLogger("setup", loglevel, false)

	if err := viper.ReadInConfig(); err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			logger.Debug(err.Error())
		} else {
			logger.Error(err.Error())
			os.Exit(1)
		}
	} else {
		msg := fmt.Sprintf("Using config: %s (loglevel: %s)", viper.ConfigFileUsed(), viper.GetString("loglevel"))
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
