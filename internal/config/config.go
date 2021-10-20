package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/internal/logger"
)

const (
	AccessTokenKey    = "access-token"
	AccessTokenEnvKey = "ACCESS_TOKEN"

	VerboseOutputKey    = "verbose"
	VerboseOutputEnvKey = "VERBOSE_OUTPUT"

	JSONOutputKey    = "json"
	JSONOutputEnvKey = "JSON_OUTPUT"

	APIBaseURL = "api_base_url"
	AppName    = "app"

	Builtinsfile    = "builtins_file"
	GQLErrorLogging = "gqlerrorlogging"
	Installer       = "installer"
	BuildKitNodeID  = "buildkit_node_id"
	WireGuardState  = "wire_guard_state"
	RegistryHost    = "registry_host"
)

func Load(logger *logger.Logger) (*viper.Viper, error) {
	v := viper.New()

	v.SetDefault(APIBaseURL, "https://api.fly.io")
	v.SetDefault(RegistryHost, "registry.fly.io")

	v.SetEnvPrefix("FLY")
	v.AutomaticEnv()

	v.BindEnv(VerboseOutputKey, VerboseOutputEnvKey)
	v.BindEnv(GQLErrorLogging, "GQLErrorLogging")

	v.SetConfigName("config")
	v.SetConfigType("yaml")

	dir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error determining user home: %v", err)
	}

	dir = filepath.Join(dir, ".fly")
	v.AddConfigPath(dir)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	logger.Debugf("config loaded from %s", v.ConfigFileUsed())

	return v, nil
}

func configDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
}
