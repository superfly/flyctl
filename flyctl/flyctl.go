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

func init() {
	InitConfig()
}

func InitConfig() {
	if err := initConfigDir(); err != nil {
		fmt.Println("Error accessing config directory at $HOME/.fly", err)
		return
	}

	initViper()
}

func ConfigDir() string {
	return configDir
}

func configFilePath() string {
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

	viper.SetDefault(ConfigAPIBaseURL, "https://fly.io")
	viper.SetDefault(ConfigRegistryHost, "registry.fly.io")

	viper.SetEnvPrefix("FLY")
	viper.AutomaticEnv()

	api.SetBaseURL(viper.GetString(ConfigAPIBaseURL))
}

func loadConfig() error {
	if configDir == "" {
		return nil
	}

	viper.SetConfigFile(configFilePath())
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err == nil {
		terminal.Debug("Loaded config from", viper.ConfigFileUsed())
		return nil
	}

	if os.IsNotExist(err) {
		if migrateLegacyConfig() {
			if err := SaveConfig(); err != nil {
				terminal.Debug("error writing config", err)
			}
		}
		return nil
	}

	return err
}

var writeableConfigKeys = []string{"access_token", "update_check"}

func SaveConfig() error {
	BackgroundTaskWG.Add(1)
	defer BackgroundTaskWG.Done()

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

	return ioutil.WriteFile(configFilePath(), data, 0644)
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
