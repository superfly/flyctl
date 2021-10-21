package config

import (
	"os"

	"github.com/superfly/flyctl/internal/env"
	"gopkg.in/yaml.v3"
)

const (
	envKeyPrefix        = "FLY_"
	accessTokenEnvKey   = envKeyPrefix + "ACCESS_TOKEN"
	verboseOutputEnvKey = envKeyPrefix + "VERBOSE_OUTPUT"
	jsonOutputEnvKey    = envKeyPrefix + "JSON_OUTPUT"

	defaultAPIBaseURL   = "https://api.fly.io"
	defaultRegistryHost = "registry.fly.io"
)

type Config struct {
	AccessToken   string `yaml:"access_token"`
	VerboseOutput bool   `yaml:"-"`
	JSONOutput    bool   `yaml:"-"`
	APIBaseURL    string `yaml:"-"`
	RegistryHost  string `yaml:"-"`
}

// New returns a new instance of Config populated with default values.
func New() *Config {
	return &Config{
		APIBaseURL:   defaultAPIBaseURL,
		RegistryHost: defaultRegistryHost,
	}
}

// ReadFromEnv sets the parts of cfg which may be controlled via environment
// variables to the values these variables contain.
func (cfg *Config) ReadEnv() {
	cfg.AccessToken = env.Get(accessTokenEnvKey)
	cfg.VerboseOutput = env.IsTruthy(verboseOutputEnvKey)
	cfg.JSONOutput = env.IsTruthy(jsonOutputEnvKey)
}

// ReadFile sets the parts of cfg which may be controlled via file to the
// values the file contains.
func (cfg *Config) ReadFile(path string) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return yaml.NewDecoder(f).Decode(cfg)
}
