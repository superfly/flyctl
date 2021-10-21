package config

import (
	"os"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/env"
)

const (
	envKeyPrefix        = "FLY_"
	accessTokenEnvKey   = envKeyPrefix + "ACCESS_TOKEN"
	apiTokenEnvKey      = envKeyPrefix + "API_TOKEN"
	verboseOutputEnvKey = envKeyPrefix + "VERBOSE"
	jsonOutputEnvKey    = envKeyPrefix + "JSON"
	logGQLEnvKey        = envKeyPrefix + "LOG_GQL_ERRORS"

	defaultAPIBaseURL   = "https://api.fly.io"
	defaultRegistryHost = "registry.fly.io"
)

type Config struct {
	APIBaseURL   string `yaml:"-"`
	RegistryHost string `yaml:"-"`

	AccessToken   string `yaml:"access_token"`
	VerboseOutput bool   `yaml:"-"`
	JSONOutput    bool   `yaml:"-"`
	LogGQLErrors  bool   `yaml:"-"`
}

// New returns a new instance of Config populated with default values.
func New() *Config {
	return &Config{
		APIBaseURL:   defaultAPIBaseURL,
		RegistryHost: defaultRegistryHost,
	}
}

// ApplyEnv sets the properties of cfg which may be set via environment
// variables to the values these variables contain.
func (cfg *Config) ApplyEnv() {
	cfg.AccessToken = env.FirstOrDefault(cfg.AccessToken,
		accessTokenEnvKey, apiTokenEnvKey)

	cfg.VerboseOutput = env.IsTruthy(verboseOutputEnvKey) || cfg.VerboseOutput
	cfg.JSONOutput = env.IsTruthy(jsonOutputEnvKey) || cfg.JSONOutput
	cfg.LogGQLErrors = env.IsTruthy(logGQLEnvKey) || cfg.LogGQLErrors
}

// ApplyFile sets the properties of cfg which may be set via configuration file
// to the values the file at the given path contains.
func (cfg *Config) ApplyFile(path string) (err error) {
	var f *os.File
	if f, err = os.Open(path); err == nil {
		err = yaml.NewDecoder(f).Decode(cfg)

		if e := f.Close(); err == nil {
			err = e
		}
	}

	return
}

// ApplyFlags sets the properties of cfg which may be set via command line flags
// to the values the flags of the given FlagSet may contain.
func (cfg *Config) ApplyFlags(fs *pflag.FlagSet) {
	applyStringFlag(&cfg.AccessToken, fs, flag.AccessToken)
	applyBoolFlag(&cfg.VerboseOutput, fs, flag.Verbose)
	applyBoolFlag(&cfg.JSONOutput, fs, flag.JSON)
}

func applyStringFlag(dst *string, fs *pflag.FlagSet, name string) {
	if !fs.Changed(name) {
		return
	}

	if v, err := fs.GetString(name); err != nil {
		panic(err)
	} else {
		*dst = v
	}
}

func applyBoolFlag(dst *bool, fs *pflag.FlagSet, name string) {
	if !fs.Changed(name) {
		return
	}

	if v, err := fs.GetBool(name); err != nil {
		panic(err)
	} else {
		*dst = v
	}
}
