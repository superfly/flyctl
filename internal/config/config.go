package config

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/task"
)

const (
	// FileName denotes the name of the config file.
	FileName = "config.yml"

	envKeyPrefix               = "FLY_"
	apiBaseURLEnvKey           = envKeyPrefix + "API_BASE_URL"
	flapsBaseURLEnvKey         = envKeyPrefix + "FLAPS_BASE_URL"
	metricsBaseURLEnvKey       = envKeyPrefix + "METRICS_BASE_URL"
	AccessTokenEnvKey          = envKeyPrefix + "ACCESS_TOKEN"
	AccessTokenFileKey         = "access_token"
	MetricsTokenEnvKey         = envKeyPrefix + "METRICS_TOKEN"
	MetricsTokenFileKey        = "metrics_token"
	SendMetricsEnvKey          = envKeyPrefix + "SEND_METRICS"
	SendMetricsFileKey         = "send_metrics"
	AutoUpdateFileKey          = "auto_update"
	WireGuardStateFileKey      = "wire_guard_state"
	WireGuardWebsocketsFileKey = "wire_guard_websockets"
	APITokenEnvKey             = envKeyPrefix + "API_TOKEN"
	orgEnvKey                  = envKeyPrefix + "ORG"
	registryHostEnvKey         = envKeyPrefix + "REGISTRY_HOST"
	organizationEnvKey         = envKeyPrefix + "ORGANIZATION"
	regionEnvKey               = envKeyPrefix + "REGION"
	verboseOutputEnvKey        = envKeyPrefix + "VERBOSE"
	jsonOutputEnvKey           = envKeyPrefix + "JSON"
	logGQLEnvKey               = envKeyPrefix + "LOG_GQL_ERRORS"
	localOnlyEnvKey            = envKeyPrefix + "LOCAL_ONLY"

	defaultAPIBaseURL     = "https://api.fly.io"
	defaultFlapsBaseURL   = "https://api.machines.dev"
	defaultRegistryHost   = "registry.fly.io"
	defaultMetricsBaseURL = "https://flyctl-metrics.fly.dev"
)

// Config wraps the functionality of the configuration file.
//
// Instances of Config are safe for concurrent use.
type Config struct {
	mu   sync.RWMutex
	path string

	watchOnce sync.Once
	watchErr  error
	subs      map[chan *Config]struct{}

	// APIBaseURL denotes the base URL of the API.
	APIBaseURL string

	// FlapsBaseURL denotes base URL for FLAPS (also known as the Machines API).
	FlapsBaseURL string

	// MetricsBaseURL denotes the base URL of the metrics API.
	MetricsBaseURL string

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
		APIBaseURL:     defaultAPIBaseURL,
		FlapsBaseURL:   defaultFlapsBaseURL,
		RegistryHost:   defaultRegistryHost,
		MetricsBaseURL: defaultMetricsBaseURL,
		Tokens:         new(tokens.Tokens),
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
	cfg.SendMetrics = env.IsTruthy(SendMetricsEnvKey) || cfg.SendMetrics
}

// applyFile sets the properties of cfg which may be set via configuration file
// to the values the file at the given path contains.
func (cfg *Config) applyFile(path string) (err error) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.path = path

	var w struct {
		AccessToken  string `yaml:"access_token"`
		MetricsToken string `yaml:"metrics_token"`
		SendMetrics  bool   `yaml:"send_metrics"`
		AutoUpdate   bool   `yaml:"auto_update"`
	}
	w.SendMetrics = true
	w.AutoUpdate = true

	if err = unmarshal(path, &w); err == nil {
		cfg.Tokens = tokens.Parse(w.AccessToken)
		cfg.Tokens.FromConfigFile = path

		cfg.MetricsToken = w.MetricsToken
		cfg.SendMetrics = w.SendMetrics
		cfg.AutoUpdate = w.AutoUpdate
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

func (cfg *Config) Watch(ctx context.Context) (chan *Config, error) {
	cfg.watchOnce.Do(func() {
		watch, err := fsnotify.NewWatcher()
		if err != nil {
			cfg.watchErr = err
			return
		}

		if err := watch.Add(cfg.path); err != nil {
			cfg.watchErr = err
			return
		}

		cfg.subs = make(map[chan *Config]struct{})

		task.FromContext(ctx).Run(func(ctx context.Context) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			cleanupDone := make(chan struct{})
			defer func() { <-cleanupDone }()

			go func() {
				defer close(cleanupDone)

				<-ctx.Done()

				cfg.mu.Lock()
				defer cfg.mu.Unlock()

				cfg.watchErr = errors.Join(cfg.watchErr, ctx.Err(), watch.Close())

				for sub := range cfg.subs {
					close(sub)
				}
				cfg.subs = nil
			}()

			var (
				notifyCtx        context.Context
				cancelNotify     context.CancelFunc  = func() {}
				cancelLastNotify *context.CancelFunc = &cancelNotify
			)
			defer func() { (*cancelLastNotify)() }()

			for {
				select {
				case e, open := <-watch.Events:
					if !open {
						return
					}

					if !e.Has(fsnotify.Write) {
						continue
					}

					// Debounce change notifications: notifySubs sleeps for 10ms
					// before notifying subs. If we get another change before
					// that, we preempt the previous notification attempt. This
					// is necessary because we receive multiple notifications
					// for a single config change on windows and the first event
					// fires before the change is available to be read.
					(*cancelLastNotify)()
					notifyCtx, cancelNotify = context.WithCancel(ctx)
					cancelLastNotify = &cancelNotify

					go cfg.notifySubs(notifyCtx)
				case err := <-watch.Errors:
					cfg.mu.Lock()
					defer cfg.mu.Unlock()

					cfg.watchErr = errors.Join(cfg.watchErr, err)

					return
				case <-ctx.Done():
					return
				}
			}
		})
	})

	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if cfg.watchErr != nil {
		return nil, cfg.watchErr
	}

	sub := make(chan *Config)
	cfg.subs[sub] = struct{}{}

	return sub, nil
}

func (cfg *Config) Unwatch(sub chan *Config) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if cfg.subs != nil {
		delete(cfg.subs, sub)
		close(sub)
	}
}

func (cfg *Config) notifySubs(ctx context.Context) {
	// sleep for 10ms to facilitate debouncing (described above)
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Millisecond):
	}

	newCfg, err := Load(ctx, cfg.path)
	if err != nil {
		return
	}

	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	// just in case we have a slow subscriber
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()

	for sub := range cfg.subs {
		select {
		case sub <- newCfg:
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
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
