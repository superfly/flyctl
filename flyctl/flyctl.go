package flyctl

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
	"gopkg.in/yaml.v2"
)

var configDir string

// InitConfig - Initialises config file for Viper
func InitConfig() {
	if err := initConfigDir(); err != nil {
		fmt.Println("Error accessing config directory at $HOME/.fly", err)
		return
	}

	initViper()
}

// ConfigDir - Returns Directory holding the Config file
func ConfigDir() string {
	return configDir
}

// ConfigFilePath - returns the path to the config file
func ConfigFilePath() string {
	return path.Join(configDir, "config.yml")
}

func initConfigDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(homeDir, ".fly")

	if !helpers.DirectoryExists(dir) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	configDir = dir

	return nil
}

func initViper() {
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config", err)
	}

	viper.SetDefault(ConfigAPIBaseURL, "https://api.fly.io")
	viper.SetDefault(ConfigRegistryHost, "registry.fly.io")

	viper.BindEnv(ConfigVerboseOutput, "VERBOSE")
	viper.BindEnv(ConfigGQLErrorLogging, "GQLErrorLogging")

	viper.SetEnvPrefix("FLY")
	viper.AutomaticEnv()

	api.SetBaseURL(viper.GetString(ConfigAPIBaseURL))
	api.SetErrorLog(viper.GetBool(ConfigGQLErrorLogging))
}

func loadConfig() error {
	if configDir == "" {
		return nil
	}

	viper.SetConfigFile(ConfigFilePath())
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err == nil {
		terminal.Debug("Loaded flyctl config from", viper.ConfigFileUsed())
		return nil
	}

	if os.IsNotExist(err) {
		if migrateLegacyConfig() {
			if err := SaveConfig(); err != nil {
				terminal.Debug("error writing flyctl config", err)
			}
		}
		return nil
	}

	return err
}

// GetAPIToken - returns the current API Token, env vars take precedence. Avoids pulling in env vars into the config.
func GetAPIToken() string {
	// Are either env vars set?
	// check Access token
	accessToken, lookup := os.LookupEnv("FLY_ACCESS_TOKEN")

	if lookup {
		return accessToken
	}

	// check API token
	apiToken, lookup := os.LookupEnv("FLY_API_TOKEN")

	if lookup {
		return apiToken
	}

	viperAuth := viper.GetString(ConfigAPIToken)

	return viperAuth

}

var writeableConfigKeys = []string{ConfigAPIToken, ConfigInstaller, ConfigWireGuardState, BuildKitNodeID}

func SaveConfig() error {
	out := map[string]interface{}{}

	for key, val := range viper.AllSettings() {
		if persistConfigKey(key) {
			out[key] = val
		}
	}

	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(ConfigFilePath(), data, 0600)
}

func persistConfigKey(key string) bool {

	if viper.InConfig(key) {
		return true
	}

	for _, k := range writeableConfigKeys {
		if k == key {
			return true
		}
	}

	return false
}

func migrateLegacyConfig() bool {
	legacy := viper.New()
	legacy.SetConfigFile(path.Join(configDir, "credentials.yml"))
	if err := legacy.ReadInConfig(); err != nil {
		return false
	}

	viper.MergeConfigMap(legacy.AllSettings())

	return true
}
