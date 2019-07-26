package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/cli/auth"
)

var FlyToken string
var FlyAPIBaseURL = "https://fly.io"

var rootCmd = &cobra.Command{
	Use:   "fly",
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
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&FlyToken, "token", "", accessToken(), "fly api token")

	if base := os.Getenv("FLY_BASE_URL"); base != "" {
		FlyAPIBaseURL = base
	}
}

func accessToken() string {
	if token := os.Getenv("FLY_ACCESS_TOKEN"); token != "" {
		return token
	}

	if accessToken, err := auth.GetSavedAccessToken(); err == nil {
		return accessToken
	}

	return ""
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// if cfgFile != "" {
	// 	// Use config file from the flag.
	// 	viper.SetConfigFile(cfgFile)
	// } else {
	// 	// Find home directory.
	// 	home, err := homedir.Dir()
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		os.Exit(1)
	// 	}

	// 	// Search config in home directory with name ".my.name" (without extension).
	// 	viper.AddConfigPath(home)
	// 	viper.SetConfigName(".my.name")
	// }

	// viper.AutomaticEnv() // read in environment variables that match

	// // If a config file is found, read it in.
	// if err := viper.ReadInConfig(); err == nil {
	// 	fmt.Println("Using config file:", viper.ConfigFileUsed())
	// }
}
