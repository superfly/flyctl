package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

var cfgFile string

var appFlag *pflag.Flag

var rootCmd = &cobra.Command{
	Short: "sort",
	Long:  `long`,
}

func Execute() {
	bindCommandFlags(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		terminal.Error(err)
		os.Exit(1)
	}
}

func bindCommandFlags(cmd *cobra.Command) {
	if cmd.HasFlags() {
		viper.BindPFlags(cmd.Flags())
	}

	if cmd.HasPersistentFlags() {
		viper.BindPFlags(cmd.PersistentFlags())
	}

	if cmd.HasSubCommands() {
		for _, subcmd := range cmd.Commands() {
			bindCommandFlags(subcmd)
		}
	}
}

func init() {
	cobra.OnInitialize(flyctl.InitConfig)
	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.RegisterAlias(flyctl.ConfigAPIAccessToken, "access-token")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
}

func addAppFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("app", "a", "", "Fly app to run command against")
}
