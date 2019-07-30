package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

var flyToken string
var flyAPIBaseURL string
var cfgFile string

var rootCmd = &cobra.Command{
	Short: "sort",
	Long:  `long`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(flyctl.InitConfig)
	rootCmd.PersistentFlags().StringP("access-token", "t", "", "Fly API Access Token")
	viper.RegisterAlias("access-token", flyctl.ConfigAPIAccessToken)
	viper.BindPFlags(rootCmd.PersistentFlags())
}
