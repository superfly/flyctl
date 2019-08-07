package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"gopkg.in/yaml.v2"
)

var rootCmd = &cobra.Command{
	Use:  "flyctl",
	Long: `flycyl is a command line interface for the Fly.io platform`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	},
}

var newRootCmd = &Command{
	Command: &cobra.Command{
		Use:  "flyctl",
		Long: `flycyl is a command line interface for the Fly.io platform`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
		},
	},
}

func Execute() {
	if err := newRootCmd.Execute(); err != nil {
		terminal.Error(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	newRootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.BindPFlag(flyctl.ConfigAPIAccessToken, newRootCmd.PersistentFlags().Lookup("access-token"))
	viper.BindEnv(flyctl.ConfigAPIAccessToken, "FLY_ACCESS_TOKEN")

	newRootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	viper.BindPFlag(flyctl.ConfigVerboseOutput, newRootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindEnv(flyctl.ConfigVerboseOutput, "VERBOSE")

	newRootCmd.AddCommand(
		newAuthCommand(),
		newAppStatusCommand(),
		newAppListCommand(),
		newAppReleasesListCommand(),
		newAppLogsCommand(),
		newAppSecretsCommand(),
		newVersionCommand(),
		newAppCreateCommand(),
		newDeployCommand(),
	)
}

func initConfig() {
	// read in credentials.yml, maybe migrate to new config?
	if err := loadConfig(); err != nil {
		panic(err)
	}

	viper.SetDefault(flyctl.ConfigAPIBaseURL, "https://fly.io")

	viper.SetEnvPrefix("FLY")
	viper.AutomaticEnv()
}

func loadConfig() error {
	configDir, err := flyctl.ConfigDir()
	if err != nil {
		return err
	}

	configFile := path.Join(configDir, "credentials.yml")

	viper.SetConfigType("yaml")
	viper.SetConfigFile(configFile)

	if _, err := os.Stat(configFile); err == nil {
		if err := viper.ReadInConfig(); err != nil {
			return err
		}
	}

	terminal.Debug("Read config file", viper.ConfigFileUsed())

	return nil
}

func saveConfig() error {
	out := map[string]string{}
	if accessToken := viper.GetString(flyctl.ConfigAPIAccessToken); accessToken != "" {
		out["access_token"] = accessToken
	}

	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return ioutil.WriteFile(viper.ConfigFileUsed(), data, 0644)
}
