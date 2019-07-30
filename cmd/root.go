package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

var cfgFile string

var appFlag *pflag.Flag

var rootCmd = &cobra.Command{
	Short: "sort",
	Long:  `long`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	bindCommandFlags(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
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
	viper.RegisterAlias("access-token", flyctl.ConfigAPIAccessToken)
	rootCmd.PersistentFlags().Bool("trace", false, "trace api access")
}

func addAppFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("app", "a", "", "Fly app to run command against")
}
