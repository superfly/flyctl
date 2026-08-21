package flyctl

import (
	"fmt"
	"os"
	"path"
	"slices"

	"github.com/spf13/viper"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/instrument"
	"github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/terminal"
	"gopkg.in/yaml.v3"
)

var configDir string

// InitConfig - Initialises config file for Viper
func InitConfig() {
	dir, err := configDirectory()
	if err != nil {
		fmt.Println("Error accessing home directory", err)

		return
	}

	if err = initConfigDir(dir); err != nil {
		fmt.Println(
			fmt.Sprintf("Error accessing config directory at %s", dir),
			err,
		)

		return
	}

	initViper()
}

// configDirectory resolves the active profile's config directory.
//
// This runs while the root command is being built, before cobra has parsed
// anything, so the profile flag is scraped straight out of os.Args. A profile
// that fails to resolve is not reported here: the same resolution runs again
// as a command preparer, which raises a far better error. Falling back to the
// plain config directory keeps this from being the thing that reports it.
func configDirectory() (string, error) {
	opts := profile.ResolveOptions{Flag: profile.FlagFromArgs(os.Args[1:])}

	if wd, err := os.Getwd(); err == nil {
		opts.WorkingDir = wd
	}

	if res, err := profile.Resolve(opts); err == nil {
		return res.Dir, nil
	}

	return helpers.GetConfigDirectory()
}

// ConfigDir - Returns Directory holding the Config file
func ConfigDir() string {
	return configDir
}

// ConfigFilePath - returns the path to the config file
func ConfigFilePath() string {
	return path.Join(configDir, "config.yml")
}

func initConfigDir(dir string) error {
	if !helpers.DirectoryExists(dir) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	configDir = dir

	return nil
}

// viper keys that shouldn't be loaded from the environment
var noEnvKeys = map[string]bool{
	ConfigAPIToken: true,
}

func initViper() {
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config", err)
	}

	viper.SetDefault(ConfigAPIBaseURL, "https://api.fly.io")
	viper.SetDefault(ConfigFlapsBaseUrl, "https://api.machines.dev")
	viper.SetDefault(ConfigRegistryHost, "registry.fly.io")
	viper.SetDefault(ConfigWireGuardWebsockets, true)
	viper.BindEnv(ConfigVerboseOutput, "VERBOSE")
	viper.BindEnv(ConfigGQLErrorLogging, "GQLErrorLogging")

	viper.SetEnvPrefix("FLY")

	for _, key := range viper.AllKeys() {
		if noEnvKeys[key] {
			continue
		}
		viper.BindEnv(key)
	}

	fly.SetBaseURL(viper.GetString(ConfigAPIBaseURL))
	fly.SetErrorLog(viper.GetBool(ConfigGQLErrorLogging))
	fly.SetInstrumenter(instrument.ApiAdapter)
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
			if err := saveConfig(); err != nil {
				terminal.Debug("error writing flyctl config", err)
			}
		}

		return nil
	}

	return err
}

var writeableConfigKeys = []string{ConfigAPIToken, ConfigInstaller, ConfigAppSecretsMinvers, ConfigWireGuardState, ConfigWireGuardWebsockets, BuildKitNodeID}

func saveConfig() error {
	out := map[string]any{}

	for key, val := range viper.AllSettings() {
		if persistConfigKey(key) {
			out[key] = val
		}
	}

	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigFilePath(), data, 0o600)
}

func persistConfigKey(key string) bool {
	if viper.InConfig(key) {
		return true
	}

	return slices.Contains(writeableConfigKeys, key)
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
