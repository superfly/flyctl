// Package app implements functionality related to reading and writing app
// configuration files.
package app

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/scanner"
)

const (
	// DefaultConfigFileName denotes the default application configuration file name.
	DefaultConfigFileName = "fly.toml"
	// Config is versioned, initially, to separate nomad from machine apps without having to consult
	// the API
	NomadPlatform    = "nomad"
	MachinesPlatform = "machines"
)

// Config wraps the properties of app configuration.
type Config struct {
	AppName       string                    `toml:"app,omitempty" json:"app,omitempty"`
	Build         *Build                    `toml:"build,omitempty" json:"build,omitempty"`
	HttpService   *HTTPService              `toml:"http_service,omitempty" json:"http_service,omitempty"`
	Definition    map[string]any            `toml:"definition,omitempty" json:"definition,omitempty"`
	Services      []Service                 `toml:"services" json:"services,omitempty"`
	Env           map[string]string         `toml:"env" json:"env,omitempty"`
	Metrics       *api.MachineMetrics       `toml:"metrics" json:"metrics,omitempty"`
	Statics       []*Static                 `toml:"statics,omitempty" json:"statics,omitempty"`
	Deploy        *Deploy                   `toml:"deploy, omitempty" json:"deploy,omitempty"`
	PrimaryRegion string                    `toml:"primary_region,omitempty" json:"primary_region,omitempty"`
	Checks        map[string]*ToplevelCheck `toml:"checks,omitempty" json:"checks,omitempty"`
	Mounts        *scanner.Volume           `toml:"mounts,omitempty" json:"mounts,omitempty"`
	Processes     map[string]string         `toml:"processes,omitempty" json:"processes,omitempty"`
	Experimental  Experimental              `toml:"experimental,omitempty" json:"experimental,omitempty"`
	FlyTomlPath   string                    `toml:"-" json:"-"`
}

type Deploy struct {
	ReleaseCommand string `toml:"release_command,omitempty"`
	Strategy       string `toml:"strategy,omitempty"`
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix" validate:"required"`
}

type VM struct {
	CpuCount int `toml:"cpu_count,omitempty"`
	Memory   int `toml:"memory,omitempty"`
}

type Build struct {
	Builder           string            `toml:"builder,omitempty"`
	Args              map[string]string `toml:"args,omitempty"`
	Buildpacks        []string          `toml:"buildpacks,omitempty"`
	Image             string            `toml:"image,omitempty"`
	Settings          map[string]any    `toml:"settings,omitempty"`
	Builtin           string            `toml:"builtin,omitempty"`
	Dockerfile        string            `toml:"dockerfile,omitempty"`
	Ignorefile        string            `toml:"ignorefile,omitempty"`
	DockerBuildTarget string            `toml:"build-target,omitempty"`
}

type Experimental struct {
	Cmd          []string `toml:"cmd,omitempty"`
	Entrypoint   []string `toml:"entrypoint,omitempty"`
	Exec         []string `toml:"exec,omitempty"`
	AutoRollback bool     `toml:"auto_rollback,omitempty"`
	EnableConsul bool     `toml:"enable_consul,omitempty"`
	EnableEtcd   bool     `toml:"enable_etcd,omitempty"`
}

func (c *Config) HasDefinition() bool {
	return len(c.Definition) > 0
}

func (c *Config) HasBuilder() bool {
	return c.Build != nil && c.Build.Builder != ""
}

func (c *Config) HasBuiltin() bool {
	return c.Build != nil && c.Build.Builtin != ""
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

func (c *Config) Image() string {
	if c.Build == nil {
		return ""
	}
	return c.Build.Image
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

// HasServices - Does this config have a services section
func (c *Config) HasServices() bool {
	_, ok := c.Definition["services"].([]any)

	return ok
}

func (c *Config) SetInternalPort(port int) bool {
	services, ok := c.Definition["services"].([]any)
	if !ok {
		return false
	}

	if len(services) == 0 {
		return false
	}

	if service, ok := services[0].(map[string]any); ok {
		service["internal_port"] = port

		return true
	}

	return false
}

func (c *Config) SetConcurrency(soft int, hard int) bool {
	services, ok := c.Definition["services"].([]any)
	if !ok || len(services) == 0 {
		return false
	}

	if service, ok := services[0].(map[string]any); ok {
		if concurrency, ok := service["concurrency"].(map[string]any); ok {
			concurrency["hard_limit"] = hard
			concurrency["soft_limit"] = soft
			return true

		}
	}

	return false
}

func (c *Config) InternalPort() (int, error) {
	tmpservices, ok := c.Definition["services"]
	if !ok {
		return -1, errors.New("could not find internal port setting")
	}

	services, ok := tmpservices.([]map[string]any)
	if ok {
		internalport, ok := services[0]["internal_port"].(int64)
		if ok {
			return int(internalport), nil
		}
		internalportfloat, ok := services[0]["internal_port"].(float64)
		if ok {
			return int(internalportfloat), nil
		}
	}
	return 8080, nil
}

func (c *Config) SetReleaseCommand(cmd string) {
	var deploy map[string]string

	if rawDeploy, ok := c.Definition["deploy"]; ok {
		if castDeploy, ok := rawDeploy.(map[string]string); ok {
			deploy = castDeploy
		}
	}

	if deploy == nil {
		deploy = map[string]string{}
	}

	deploy["release_command"] = cmd

	c.Definition["deploy"] = deploy
}

func (c *Config) SetDockerCommand(cmd string) {
	var experimental map[string]string

	if rawExperimental, ok := c.Definition["experimental"]; ok {
		if castExperimental, ok := rawExperimental.(map[string]string); ok {
			experimental = castExperimental
		}
	}

	if experimental == nil {
		experimental = map[string]string{}
	}

	experimental["cmd"] = cmd

	c.Definition["experimental"] = experimental
}

func (c *Config) SetKillSignal(signal string) {
	c.Definition["kill_signal"] = signal
}

func (c *Config) SetDockerEntrypoint(entrypoint string) {
	var experimental map[string]string

	if rawExperimental, ok := c.Definition["experimental"]; ok {
		if castExperimental, ok := rawExperimental.(map[string]string); ok {
			experimental = castExperimental
		}
	}

	if experimental == nil {
		experimental = map[string]string{}
	}

	experimental["entrypoint"] = entrypoint

	c.Definition["experimental"] = experimental
}

func (c *Config) SetEnvVariable(name, value string) {
	c.SetEnvVariables(map[string]string{name: value})
}

func (c *Config) SetEnvVariables(vals map[string]string) {
	env := c.GetEnvVariables()

	for k, v := range vals {
		env[k] = v
	}

	c.Definition["env"] = env
}

func (c *Config) GetEnvVariables() map[string]string {
	env := map[string]string{}

	if rawEnv, ok := c.Definition["env"]; ok {
		// we get map[string]any when unmarshaling toml, and map[string]string from SetEnvVariables. Support them both :vomit:
		switch castEnv := rawEnv.(type) {
		case map[string]string:
			env = castEnv
		case map[string]any:
			for k, v := range castEnv {
				if stringVal, ok := v.(string); ok {
					env[k] = stringVal
				} else {
					env[k] = fmt.Sprintf("%v", v)
				}
			}
		}
	}

	return env
}

func (c *Config) SetProcess(name, value string) {
	var processes map[string]string

	if rawProcesses, ok := c.Definition["processes"]; ok {
		if castProcesses, ok := rawProcesses.(map[string]string); ok {
			processes = castProcesses
		}
	}

	if processes == nil {
		processes = map[string]string{}
	}

	processes[name] = value

	c.Definition["processes"] = processes
}

func (c *Config) SetStatics(statics []scanner.Static) {
	c.Definition["statics"] = statics
}

func (c *Config) SetVolumes(volumes []scanner.Volume) {
	c.Definition["mounts"] = volumes
}

func (c *Config) Validate() (err error) {
	Validator := validator.New()
	Validator.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		// skip if tag key says it should be ignored
		if name == "-" {
			return ""
		}
		return name
	})

	err = Validator.Struct(c)

	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			if err.Tag() == "required" {
				fmt.Printf("%s is required\n", err.Field())
			} else {
				fmt.Printf("Validation error on %s: %s\n", err.Field(), err.Tag())
			}
		}
	}
	return
}
