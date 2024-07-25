package config

import (
	"context"
	"errors"
	"io/fs"
	"sync"

	"github.com/spf13/pflag"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

const (
	// FileName denotes the name of the config file.
	FileName = "config.yml"

	apiBaseURLEnvKey           = "FLY_API_BASE_URL"
	flapsBaseURLEnvKey         = "FLY_FLAPS_BASE_URL"
	metricsBaseURLEnvKey       = "FLY_METRICS_BASE_URL"
	syntheticsBaseURLEnvKey    = "FLY_SYNTHETICS_BASE_URL"
	AccessTokenEnvKey          = "FLY_ACCESS_TOKEN"
	AccessTokenFileKey         = "access_token"
	MetricsTokenEnvKey         = "FLY_METRICS_TOKEN"
	MetricsTokenFileKey        = "metrics_token"
	SendMetricsEnvKey          = "FLY_SEND_METRICS"
	SyntheticsAgentEnvKey      = "FLY_SYNTHETICS_AGENT"
	SendMetricsFileKey         = "send_metrics"
	SyntheticsAgentFileKey     = "synthetics_agent"
	AutoUpdateFileKey          = "auto_update"
	WireGuardStateFileKey      = "wire_guard_state"
	WireGuardWebsocketsFileKey = "wire_guard_websockets"
	APITokenEnvKey             = "FLY_API_TOKEN"
	orgEnvKey                  = "FLY_ORG"
	registryHostEnvKey         = "FLY_REGISTRY_HOST"
	organizationEnvKey         = "FLY_ORGANIZATION"
	regionEnvKey               = "FLY_REGION"
	verboseOutputEnvKey        = "FLY_VERBOSE"
	jsonOutputEnvKey           = "FLY_JSON"
	logGQLEnvKey               = "FLY_LOG_GQL_ERRORS"
	localOnlyEnvKey            = "FLY_LOCAL_ONLY"

	defaultAPIBaseURL        = "https://api.fly.io"
	defaultFlapsBaseURL      = "https://api.machines.dev"
	defaultRegistryHost      = "registry.fly.io"
	defaultMetricsBaseURL    = "https://flyctl-metrics.fly.dev"
	defaultSyntheticsBaseURL = "https://flynthetics.fly.dev"
)

// Config wraps the functionality of the configuration file.
//
// Instances of Config are safe for concurrent use.
type Config struct {
	mu sync.RWMutex

	// APIBaseURL denotes the base URL of the API.
	APIBaseURL string

	// FlapsBaseURL denotes base URL for FLAPS (also known as the Machines API).
	FlapsBaseURL string

	// MetricsBaseURL denotes the base URL of the metrics API.
	MetricsBaseURL string

	// SyntheticsBaseURL denotes the base URL of the synthetics API.
	SyntheticsBaseURL string

	// RegistryHost denotes the docker registry host.
	RegistryHost string

	// VerboseOutput denotes whether the user wants the output to be verbose.
	VerboseOutput bool

	// JSONOutput denotes whether the user wants the output to be JSON.
	JSONOutput bool

	// LogGQLErrors denotes whether the user wants the log GraphQL errors.
	LogGQLErrors bool

	// SendMetrics denotes whether the user wants to send metrics.
	SendMetrics bool

	// SyntheticsAgent denotes whether the user wants to run the synthetics monitoring agent.
	SyntheticsAgent bool

	// AutoUpdate denotes whether the user wants to automatically update flyctl.
	AutoUpdate bool

	// Organization denotes the organizational slug the user has selected.
	Organization string

	// Region denotes the region slug the user has selected.
	Region string

	// LocalOnly denotes whether the user wants only local operations.
	LocalOnly bool

	// Tokens is the user's authentication token(s). They are used differently
	// depending on where they need to be sent.
	Tokens *tokens.Tokens

	// MetricsToken denotes the user's metrics token.
	MetricsToken string
}

func Load(ctx context.Context, path string) (*Config, error) {
	cfg := &Config{
		APIBaseURL:        defaultAPIBaseURL,
		FlapsBaseURL:      defaultFlapsBaseURL,
		RegistryHost:      defaultRegistryHost,
		MetricsBaseURL:    defaultMetricsBaseURL,
		SyntheticsBaseURL: defaultSyntheticsBaseURL,
		Tokens:            new(tokens.Tokens),
	}

	// Apply config from the config file, if it exists
	if err := cfg.applyFile(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Apply config from the environment, overriding anything from the file
	cfg.applyEnv()

	// Finally, apply command line options, overriding any previous setting
	cfg.applyFlags(flagctx.FromContext(ctx))

	return cfg, nil
}

// applyEnv sets the properties of cfg which may be set via environment
// variables to the values these variables contain.
//
// applyEnv does not change the dirty state of config.
func (cfg *Config) applyEnv() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if token := env.First(AccessTokenEnvKey, APITokenEnvKey); token != "" {
		cfg.Tokens = tokens.Parse(token)
	}

	cfg.VerboseOutput = env.IsTruthy(verboseOutputEnvKey) || cfg.VerboseOutput
	cfg.JSONOutput = env.IsTruthy(jsonOutputEnvKey) || cfg.JSONOutput
	cfg.LogGQLErrors = env.IsTruthy(logGQLEnvKey) || cfg.LogGQLErrors
	cfg.LocalOnly = env.IsTruthy(localOnlyEnvKey) || cfg.LocalOnly

	cfg.Organization = env.FirstOrDefault(cfg.Organization,
		orgEnvKey, organizationEnvKey)
	cfg.Region = env.FirstOrDefault(cfg.Region, regionEnvKey)
	cfg.RegistryHost = env.FirstOrDefault(cfg.RegistryHost, registryHostEnvKey)
	cfg.APIBaseURL = env.FirstOrDefault(cfg.APIBaseURL, apiBaseURLEnvKey)
	cfg.FlapsBaseURL = env.FirstOrDefault(cfg.FlapsBaseURL, flapsBaseURLEnvKey)
	cfg.MetricsBaseURL = env.FirstOrDefault(cfg.MetricsBaseURL, metricsBaseURLEnvKey)
	cfg.SyntheticsBaseURL = env.FirstOrDefault(cfg.SyntheticsBaseURL, syntheticsBaseURLEnvKey)
	cfg.SendMetrics = env.IsTruthy(SendMetricsEnvKey) || cfg.SendMetrics
	cfg.SyntheticsAgent = env.IsTruthy(SyntheticsAgentEnvKey) || cfg.SyntheticsAgent
}

// applyFile sets the properties of cfg which may be set via configuration file
// to the values the file at the given path contains.
func (cfg *Config) applyFile(path string) (err error) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	var w struct {
		AccessToken     string `yaml:"access_token"`
		MetricsToken    string `yaml:"metrics_token"`
		SendMetrics     bool   `yaml:"send_metrics"`
		AutoUpdate      bool   `yaml:"auto_update"`
		SyntheticsAgent bool   `yaml:"synthetics_agent"`
	}
	w.SendMetrics = true
	w.AutoUpdate = true
	w.SyntheticsAgent = true

	if err = unmarshal(path, &w); err == nil {
		cfg.Tokens = tokens.ParseFromFile(w.AccessToken, path)
		cfg.MetricsToken = w.MetricsToken
		cfg.SendMetrics = w.SendMetrics
		cfg.AutoUpdate = w.AutoUpdate
		cfg.SyntheticsAgent = w.SyntheticsAgent
	}

	return
}

// applyFlags sets the properties of cfg which may be set via command line flags
// to the values the flags of the given FlagSet may contain.
func (cfg *Config) applyFlags(fs *pflag.FlagSet) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	applyStringFlags(fs, map[string]*string{
		flagnames.Org:    &cfg.Organization,
		flagnames.Region: &cfg.Region,
	})

	applyBoolFlags(fs, map[string]*bool{
		flagnames.Verbose:    &cfg.VerboseOutput,
		flagnames.JSONOutput: &cfg.JSONOutput,
		flagnames.LocalOnly:  &cfg.LocalOnly,
	})

	if fs.Changed(flagnames.AccessToken) {
		if v, err := fs.GetString(flagnames.AccessToken); err != nil {
			panic(err)
		} else {
			cfg.Tokens = tokens.Parse(v)
		}
	}
}

func (cfg *Config) MetricsBaseURLIsProduction() bool {
	return cfg.MetricsBaseURL == defaultMetricsBaseURL
}

func (cfg *Config) SyntheticsBaseURLIsProduction() bool {
	return cfg.SyntheticsBaseURL == defaultSyntheticsBaseURL
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
