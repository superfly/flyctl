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
	orgEnvKey           = envKeyPrefix + "ORG"
	organizationEnvKey  = envKeyPrefix + "ORGANIZATION"
	verboseOutputEnvKey = envKeyPrefix + "VERBOSE"
	jsonOutputEnvKey    = envKeyPrefix + "JSON"
	logGQLEnvKey        = envKeyPrefix + "LOG_GQL_ERRORS"
	localOnlyEnvKey     = envKeyPrefix + "LOCAL_ONLY"

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
	Organization  string `yaml:"-"`
	LocalOnly     bool   `yaml:"-"`
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
	cfg.LocalOnly = env.IsTruthy(localOnlyEnvKey) || cfg.LocalOnly

	cfg.Organization = env.FirstOrDefault(cfg.Organization,
		orgEnvKey, organizationEnvKey)
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
	applyStringFlags(fs, map[string]*string{
		flag.AccessTokenName: &cfg.AccessToken,
		flag.OrgName:         &cfg.Organization,
	})

	applyBoolFlags(fs, map[string]*bool{
		flag.VerboseName:    &cfg.VerboseOutput,
		flag.JSONOutputName: &cfg.JSONOutput,
		flag.LocalOnlyName:  &cfg.LocalOnly,
	})
}

func applyStringFlags(fs *pflag.FlagSet, flags map[string]*string) {
	for name, dst := range flags {
		if !fs.Changed(name) {
			continue
		}

		if v, err := fs.GetString(name); err != nil {
			panic(err)
		} else {
			*dst = v
		}
	}
}

func applyBoolFlags(fs *pflag.FlagSet, flags map[string]*bool) {
	for name, dst := range flags {
		if !fs.Changed(name) {
			continue
		}

		if v, err := fs.GetBool(name); err != nil {
			panic(err)
		} else {
			*dst = v
		}
	}
}
