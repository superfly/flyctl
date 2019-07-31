package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

var rootCmd = &cobra.Command{
	Use:  "flyctl",
	Long: `flycyl is a command line interface for the Fly.io platform`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	},
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
}

func init() {
	cobra.OnInitialize(flyctl.InitConfig)
	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.RegisterAlias(flyctl.ConfigAPIAccessToken, "access-token")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")

	rootCmd.AddCommand(newAuthCommand())
	rootCmd.AddCommand(newAppCreateCommand())
	rootCmd.AddCommand(newAppListCommand())
	rootCmd.AddCommand(newAppStatusCommand())
	rootCmd.AddCommand(newAppDeployCommand())
	rootCmd.AddCommand(newAppSecretsCommand())
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newAppDeploymentsListCommand())
}
