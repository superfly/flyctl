package flyctl

import (
	"github.com/spf13/viper"
)

const (
	ConfigAPIToken        = "access_token"
	ConfigAPIBaseURL      = "api_base_url"
	ConfigAppName         = "app"
	ConfigVerboseOutput   = "verbose"
	ConfigJSONOutput      = "json"
	ConfigBuiltinsfile    = "builtins_file"
	ConfigGQLErrorLogging = "gqlerrorlogging"
	ConfigInstaller       = "installer"

	ConfigWireGuardState = "wire_guard_state"

	ConfigRegistryHost             = "registry_host"
	ConfigUpdateCheck              = "update_check"
	ConfigUpdateCheckLatestVersion = ConfigUpdateCheck + ".latest_version"
	ConfigUpdateCheckTimestamp     = ConfigUpdateCheck + ".timestamp"
	ConfigUpdateCheckOptOut        = ConfigUpdateCheck + ".opt_out"
)

const NSRoot = "flyctl"

type Config interface {
	GetString(key string) (string, error)
	GetBool(key string) bool
	GetStringSlice(key string) []string
	GetInt(key string) int
	IsSet(key string) bool
}

type config struct {
	ns string
}

func (cfg *config) nsKey(key string) string {
	if cfg.ns == NSRoot {
		return key
	}
	return cfg.ns + "." + key
}

func (cfg *config) GetString(key string) (string, error) {
	fullKey := cfg.nsKey(key)

	val := viper.GetString(fullKey)
	// required check
	return val, nil
}

func (cfg *config) GetBool(key string) bool {
	fullKey := cfg.nsKey(key)

	return viper.GetBool(fullKey)
}

func (cfg *config) GetStringSlice(key string) []string {
	fullKey := cfg.nsKey(key)

	return viper.GetStringSlice(fullKey)
}

func (cfg *config) GetInt(key string) int {
	fullKey := cfg.nsKey(key)

	return viper.GetInt(fullKey)
}

func (cfg *config) IsSet(key string) bool {
	fullKey := cfg.nsKey(key)

	return viper.IsSet(fullKey)
}

func ConfigNS(ns string) Config {
	return &config{ns}
}

var FlyConfig Config = ConfigNS(NSRoot)
