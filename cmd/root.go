package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var FlyToken string

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

	rootCmd.PersistentFlags().StringVarP(&FlyToken, "token", "", storedAccessToken(), "fly api token")
}

func storedAccessToken() string {
	if accessToken, err := readCredentialsFile(); err == nil {
		return accessToken
	}

	return os.Getenv("FLY_ACCESS_TOKEN")
}

func readCredentialsFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	credentials := filepath.Join(homeDir, ".fly", "credentials.yml")
	data, err := ioutil.ReadFile(credentials)
	if err != nil {
		return "", err
	}

	var credentialsData map[string]string
	err = yaml.Unmarshal([]byte(data), &credentialsData)
	if err != nil {
		return "", err
	}

	return credentialsData["access_token"], nil
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
