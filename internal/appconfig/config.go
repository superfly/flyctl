// Package app implements functionality related to reading and writing app
// configuration files.
package appconfig

import "github.com/superfly/flyctl/api"

const (
	// DefaultConfigFileName denotes the default application configuration file name.
	DefaultConfigFileName = "fly.toml"
	// Config is versioned, initially, to separate nomad from machine apps without having to consult
	// the API
	MachinesPlatform = "machines"
	NomadPlatform    = "nomad"
)

func NewConfig() *Config {
	return &Config{
		RawDefinition: map[string]any{},
	}
}

// Config wraps the properties of app configuration.
// NOTE: If you any new setting here, please also add a value for it at testdata/rull-reference.toml
type Config struct {
	AppName       string                    `toml:"app,omitempty" json:"app,omitempty"`
	KillSignal    string                    `toml:"kill_signal,omitempty" json:"kill_signal,omitempty"`
	KillTimeout   int                       `toml:"kill_timeout,omitempty" json:"kill_timeout,omitempty"`
	PrimaryRegion string                    `toml:"primary_region,omitempty" json:"primary_region,omitempty"`
	Experimental  *Experimental             `toml:"experimental,omitempty" json:"experimental,omitempty"`
	Build         *Build                    `toml:"build,omitempty" json:"build,omitempty"`
	Deploy        *Deploy                   `toml:"deploy, omitempty" json:"deploy,omitempty"`
	Env           map[string]string         `toml:"env,omitempty" json:"env,omitempty"`
	HttpService   *HTTPService              `toml:"http_service,omitempty" json:"http_service,omitempty"`
	Metrics       *api.MachineMetrics       `toml:"metrics,omitempty" json:"metrics,omitempty"`
	Statics       []Static                  `toml:"statics,omitempty" json:"statics,omitempty"`
	Mounts        *Volume                   `toml:"mounts,omitempty" json:"mounts,omitempty"`
	Processes     map[string]string         `toml:"processes,omitempty" json:"processes,omitempty"`
	Checks        map[string]*ToplevelCheck `toml:"checks,omitempty" json:"checks,omitempty"`
	Services      []Service                 `toml:"services,omitempty" json:"services,omitempty"`

	// RawDefinition contains fly.toml parsed as-is
	// If you add any config field that is v2 specific, be sure to remove it in SanitizeDefinition()
	RawDefinition map[string]any `toml:"-" json:"-"`

	// Path to application configuration file, usually fly.toml.
	configFilePath string

	// Indicates the intended platform to use: machines or nomad
	platformVersion string

	// Set when it fails to unmarshal fly.toml into Config
	// Don't hard fail because RawDefinition still holds the app configuration for Nomad apps
	v2UnmarshalError error
}

type Deploy struct {
	ReleaseCommand string `toml:"release_command,omitempty" json:"release_command,omitempty"`
	Strategy       string `toml:"strategy,omitempty" json:"strategy,omitempty"`
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path,omitempty" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix,omitempty" validate:"required"`
}

type Volume struct {
	Source      string `toml:"source,omitempty" json:"source,omitempty"`
	Destination string `toml:"destination" json:"destination,omitempty"`
}

type VM struct {
	CpuCount int `toml:"cpu_count,omitempty" json:"cpu_count,omitempty"`
	Memory   int `toml:"memory,omitempty" json:"memory,omitempty"`
}

type Build struct {
	Builder           string            `toml:"builder,omitempty" json:"builder,omitempty"`
	Args              map[string]string `toml:"args,omitempty" json:"args,omitempty"`
	Buildpacks        []string          `toml:"buildpacks,omitempty" json:"buildpacks,omitempty"`
	Image             string            `toml:"image,omitempty" json:"image,omitempty"`
	Settings          map[string]any    `toml:"settings,omitempty" json:"settings,omitempty"`
	Builtin           string            `toml:"builtin,omitempty" json:"builtin,omitempty"`
	Dockerfile        string            `toml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Ignorefile        string            `toml:"ignorefile,omitempty" json:"ignorefile,omitempty"`
	DockerBuildTarget string            `toml:"build-target,omitempty" json:"build-target,omitempty"`
}

type Experimental struct {
	Cmd          []string `toml:"cmd,omitempty" json:"cmd,omitempty"`
	Entrypoint   []string `toml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Exec         []string `toml:"exec,omitempty" json:"exec,omitempty"`
	AutoRollback bool     `toml:"auto_rollback,omitempty" json:"auto_rollback,omitempty"`
	EnableConsul bool     `toml:"enable_consul,omitempty" json:"enable_consul,omitempty"`
	EnableEtcd   bool     `toml:"enable_etcd,omitempty" json:"enable_etcd,omitempty"`
}

func (c *Config) ConfigFilePath() string {
	return c.configFilePath
}

func (c *Config) HasNonHttpAndHttpsStandardServices() bool {
	for _, service := range c.Services {
		switch service.Protocol {
		case "udp":
			return true
		case "tcp":
			for _, p := range service.Ports {
				if p.HasNonHttpPorts() {
					return true
				} else if p.ContainsPort(80) && (len(p.Handlers) != 1 || p.Handlers[0] != "http") {
					return true
				} else if p.ContainsPort(443) && (len(p.Handlers) != 2 || p.Handlers[0] != "tls" || p.Handlers[1] != "http") {
					return true
				}
			}
		}
	}
	return false
}

func (c *Config) HasUdpService() bool {
	for _, service := range c.Services {
		if service.Protocol == "udp" {
			return true
		}
	}
	return false
}

func (c *Config) Dockerfile() string {
	if c.Build == nil {
		return ""
	}
	return c.Build.Dockerfile
}

func (c *Config) Ignorefile() string {
	if c.Build == nil {
		return ""
	}
	return c.Build.Ignorefile
}

func (c *Config) DockerBuildTarget() string {
	if c.Build == nil {
		return ""
	}
	return c.Build.DockerBuildTarget
}
