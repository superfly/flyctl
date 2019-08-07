package flyctl

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/helpers"
)

const (
	ConfigAPIAccessToken = "access_token"
	ConfigAPIBaseURL     = "api_base_url"
	ConfigAppName        = "app"
	ConfigVerboseOutput  = "verbose"
)

const NSRoot = "flyctl"

type Config interface {
	GetString(key string) (string, error)
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

func ConfigNS(ns string) Config {
	return &config{ns}
}

var FlyConfig Config = ConfigNS(NSRoot)

func ConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".fly"), nil
}

func EnsureConfigDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	if helpers.DirectoryExists(dir) {
		return dir, nil
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	return dir, nil
}
