package config

import (
	"sync"

	"github.com/spf13/pflag"

	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/env"
)

const (
	// FileName denotes the name of the config file.
	FileName = "config.yml"

	envKeyPrefix        = "FLY_"
	apiBaseURLEnvKey    = envKeyPrefix + "API_BASE_URL"
	accessTokenEnvKey   = envKeyPrefix + "ACCESS_TOKEN"
	apiTokenEnvKey      = envKeyPrefix + "API_TOKEN"
	orgEnvKey           = envKeyPrefix + "ORG"
	registryHostEnvKey  = envKeyPrefix + "REGISTRY_HOST"
	organizationEnvKey  = envKeyPrefix + "ORGANIZATION"
	verboseOutputEnvKey = envKeyPrefix + "VERBOSE"
	jsonOutputEnvKey    = envKeyPrefix + "JSON"
	logGQLEnvKey        = envKeyPrefix + "LOG_GQL_ERRORS"
	localOnlyEnvKey     = envKeyPrefix + "LOCAL_ONLY"

	defaultAPIBaseURL   = "https://api.fly.io"
	defaultRegistryHost = "registry.fly.io"
)

// Config wraps the functionality of the configuration file.
//
// Instances of Config are safe for concurrent use.
type Config struct {
	mu sync.RWMutex

	// APIBaseURL denotes the base URL of the API.
	APIBaseURL string

	// RegistryHost denotes the docker registry host.
	RegistryHost string

	// VerboseOutput denotes whether the user wants the output to be verbose.
	VerboseOutput bool

	// JSONOutput denotes whether the user wants the output to be JSON.
	JSONOutput bool

	// LogGQLErrors denotes whether the user wants the log GraphQL errors.
	LogGQLErrors bool

	// Organization denotes the organizational slug the user has selected.
	Organization string

	// LocalOnly denotes whether the user wants only local operations.
	LocalOnly bool

	// AccessToken denotes the user's access token.
	AccessToken string
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
//
// ApplyEnv does not change the dirty state of config.
func (cfg *Config) ApplyEnv() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.AccessToken = env.FirstOrDefault(cfg.AccessToken,
		AccessTokenEnvKey, APITokenEnvKey)

	cfg.VerboseOutput = env.IsTruthy(verboseOutputEnvKey) || cfg.VerboseOutput
	cfg.JSONOutput = env.IsTruthy(jsonOutputEnvKey) || cfg.JSONOutput
	cfg.LogGQLErrors = env.IsTruthy(logGQLEnvKey) || cfg.LogGQLErrors
	cfg.LocalOnly = env.IsTruthy(localOnlyEnvKey) || cfg.LocalOnly

	cfg.Organization = env.FirstOrDefault(cfg.Organization,
		orgEnvKey, organizationEnvKey)
	cfg.RegistryHost = env.FirstOrDefault(cfg.RegistryHost, registryHostEnvKey)
	cfg.APIBaseURL = env.FirstOrDefault(cfg.APIBaseURL, apiBaseURLEnvKey)
}

// ApplyFile sets the properties of cfg which may be set via configuration file
// to the values the file at the given path contains.
func (cfg *Config) ApplyFile(path string) (err error) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	var w struct {
		AccessToken string `yaml:"access_token"`
	}

	if err = unmarshal(path, &w); err == nil {
		cfg.AccessToken = w.AccessToken
	}

	return
}

// ApplyFlags sets the properties of cfg which may be set via command line flags
// to the values the flags of the given FlagSet may contain.
func (cfg *Config) ApplyFlags(fs *pflag.FlagSet) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

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
