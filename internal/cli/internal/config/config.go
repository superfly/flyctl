package config

import (
	"bytes"
	"os"
	"sync"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

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

type wrapper struct {
	mu sync.RWMutex

	apiBaseURL    string
	registryHost  string
	verboseOutput bool
	jsonOutput    bool
	logGQLErrors  bool
	organization  string
	localOnly     bool

	dirty       bool
	accessToken string
}

// New returns a new instance of Config populated with default values.
func New() Config {
	return &wrapper{
		apiBaseURL:   defaultAPIBaseURL,
		registryHost: defaultRegistryHost,
	}
}

// Config wraps the functionality of the configuration file.
//
// Instances of Config are safe for concurrent use.
type Config interface {
	// APIBaseURL reports the base API URL.
	APIBaseURL() string

	// RegistryHost reports the docker registry host.
	RegistryHost() string

	// VerboseOutput reports whether the user wants the output to be verbose.
	VerboseOutput() bool

	// JSONOutput reports whether the user wants the output to be JSON.
	JSONOutput() bool

	// LogGQLErrors reports whether the user wants the log GraphQL errors.
	LogGQLErrors() bool

	// Organization reports the organizational slug the user has selected.
	Organization() string

	// LocalOnly reports whether the user wants only local operations.
	LocalOnly() bool

	// AccessToken reports the user's token.
	AccessToken() string

	// SetAccessToken sets the user's access token.
	SetAccessToken(string)

	// ApplyEnv sets the properties of cfg which may be set via environment
	// variables to the values these variables contain.
	ApplyEnv()

	// ApplyFile sets the properties of cfg which may be set via configuration file
	// to the values the file at the given path contains.
	ApplyFile(path string) error

	// ApplyFlags sets the properties of cfg which may be set via command line flags
	// to the values the flags of the given FlagSet may contain.
	ApplyFlags(*pflag.FlagSet)

	// Dirty reports whether the configuration should be persisted to disk.
	Dirty() bool

	// Save writes the YAML-encoded representation of the configuration to the
	// named file via os.WriteFile.
	Save(string) error

	// SaveIfDirty saves the configuration, via Save, to the named file only if
	// the configuration is dirty at the time of the call.
	SaveIfDirty(string) error
}

func (w *wrapper) APIBaseURL() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.apiBaseURL
}

func (w *wrapper) RegistryHost() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.registryHost
}

func (w *wrapper) VerboseOutput() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.verboseOutput
}

func (w *wrapper) JSONOutput() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.jsonOutput
}

func (w *wrapper) LogGQLErrors() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.logGQLErrors
}

func (w *wrapper) Organization() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.organization
}

func (w *wrapper) LocalOnly() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.localOnly
}

func (w *wrapper) AccessToken() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.accessToken
}

func (w *wrapper) SetAccessToken(token string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.dirty = true
	w.accessToken = token
}

// ApplyEnv sets the properties of cfg which may be set via environment
// variables to the values these variables contain.
func (w *wrapper) ApplyEnv() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.accessToken = env.FirstOrDefault(w.accessToken,
		AccessTokenEnvKey, APITokenEnvKey)

	w.verboseOutput = env.IsTruthy(verboseOutputEnvKey) || w.verboseOutput
	w.jsonOutput = env.IsTruthy(jsonOutputEnvKey) || w.jsonOutput
	w.logGQLErrors = env.IsTruthy(logGQLEnvKey) || w.logGQLErrors
	w.localOnly = env.IsTruthy(localOnlyEnvKey) || w.localOnly

	w.organization = env.FirstOrDefault(w.organization,
		orgEnvKey, organizationEnvKey)
	cfg.RegistryHost = env.FirstOrDefault(cfg.RegistryHost, registryHostEnvKey)
	cfg.APIBaseURL = env.FirstOrDefault(cfg.APIBaseURL, apiBaseURLEnvKey)
}

func (w *wrapper) ApplyFile(path string) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var f *os.File
	if f, err = os.Open(path); err == nil {
		err = yaml.NewDecoder(f).Decode(w)

		if e := f.Close(); err == nil {
			err = e
		}
	}

	return
}

func (w *wrapper) ApplyFlags(fs *pflag.FlagSet) {
	w.mu.Lock()
	defer w.mu.Unlock()

	applyStringFlags(fs, map[string]*string{
		flag.AccessTokenName: &w.accessToken,
		flag.OrgName:         &w.organization,
	})

	applyBoolFlags(fs, map[string]*bool{
		flag.VerboseName:    &w.verboseOutput,
		flag.JSONOutputName: &w.jsonOutput,
		flag.LocalOnlyName:  &w.localOnly,
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

func (w *wrapper) Dirty() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.dirty
}

func (w *wrapper) Save(path string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.save(path)
}

func (w *wrapper) SaveIfDirty(path string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if !w.dirty {
		return nil
	}

	return w.save(path)
}

func (w *wrapper) save(path string) (err error) {
	var b bytes.Buffer

	y := map[string]interface{}{
		"access_token": w.accessToken,
	}

	if err = yaml.NewEncoder(&b).Encode(y); err == nil {
		// TODO: this is prone to race conditions and os.WriteFile does not flush
		err = os.WriteFile(path, b.Bytes(), 0600)
	}

	return
}
