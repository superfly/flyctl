// Package appconfig implements functionality related to reading and writing app
// configuration files.
package appconfig

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"slices"

	fly "github.com/superfly/fly-go"
)

const (
	// DefaultConfigFileName denotes the default application configuration file name.
	DefaultConfigFileName = "fly.toml"
)

type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "always"
	RestartPolicyNever     RestartPolicy = "never"
	RestartPolicyOnFailure RestartPolicy = "on-failure"
)

func NewConfig() *Config {
	return &Config{
		defaultGroupName: fly.MachineProcessGroupApp,
		configFilePath:   "--config path unset--",
	}
}

// Config wraps the properties of app configuration.
// NOTE: If you any new setting here, please also add a value for it at testdata/rull-reference.toml
type Config struct {
	AppName        string        `toml:"app,omitempty" json:"app,omitempty"`
	PrimaryRegion  string        `toml:"primary_region,omitempty" json:"primary_region,omitempty"`
	KillSignal     *string       `toml:"kill_signal,omitempty" json:"kill_signal,omitempty"`
	KillTimeout    *fly.Duration `toml:"kill_timeout,omitempty" json:"kill_timeout,omitempty"`
	SwapSizeMB     *int          `toml:"swap_size_mb,omitempty" json:"swap_size_mb,omitempty"`
	ConsoleCommand string        `toml:"console_command,omitempty" json:"console_command,omitempty"`

	// Sections that are typically short and benefit from being on top
	Experimental *Experimental     `toml:"experimental,omitempty" json:"experimental,omitempty"`
	Build        *Build            `toml:"build,omitempty" json:"build,omitempty"`
	Deploy       *Deploy           `toml:"deploy,omitempty" json:"deploy,omitempty"`
	Env          map[string]string `toml:"env,omitempty" json:"env,omitempty"`

	// Fields that are process group aware must come after Processes
	Processes        map[string]string         `toml:"processes,omitempty" json:"processes,omitempty"`
	Mounts           []Mount                   `toml:"mounts,omitempty" json:"mounts,omitempty"`
	HTTPService      *HTTPService              `toml:"http_service,omitempty" json:"http_service,omitempty"`
	Services         []Service                 `toml:"services,omitempty" json:"services,omitempty"`
	Checks           map[string]*ToplevelCheck `toml:"checks,omitempty" json:"checks,omitempty"`
	Files            []File                    `toml:"files,omitempty" json:"files,omitempty"`
	HostDedicationID string                    `toml:"host_dedication_id,omitempty" json:"host_dedication_id,omitempty"`

	MachineChecks []*ServiceMachineCheck `toml:"machine_checks,omitempty" json:"machine_checks,omitempty"`

	Restart []Restart `toml:"restart,omitempty" json:"restart,omitempty"`

	Compute []*Compute `toml:"vm,omitempty" json:"vm,omitempty"`

	// Others, less important.
	Statics []Static   `toml:"statics,omitempty" json:"statics,omitempty"`
	Metrics []*Metrics `toml:"metrics,omitempty" json:"metrics,omitempty"`

	// MergedFiles is a list of files that have been merged from the app config and flags.
	MergedFiles []*fly.File `toml:"-" json:"-"`

	// Path to application configuration file, usually fly.toml.
	configFilePath string

	// Set when it fails to unmarshal fly.toml into Config
	v2UnmarshalError error

	// The default group name to refer to (used with flatten configs)
	defaultGroupName string
}

type Metrics struct {
	*fly.MachineMetrics
	Processes []string `json:"processes,omitempty" toml:"processes,omitempty"`
}

type Deploy struct {
	Strategy              string        `toml:"strategy,omitempty" json:"strategy,omitempty"`
	MaxUnavailable        *float64      `toml:"max_unavailable,omitempty" json:"max_unavailable,omitempty"`
	WaitTimeout           *fly.Duration `toml:"wait_timeout,omitempty" json:"wait_timeout,omitempty"`
	ReleaseCommand        string        `toml:"release_command,omitempty" json:"release_command,omitempty"`
	ReleaseCommandTimeout *fly.Duration `toml:"release_command_timeout,omitempty" json:"release_command_timeout,omitempty"`
	ReleaseCommandCompute *Compute      `toml:"release_command_vm,omitempty" json:"release_command_vm,omitempty"`
	SeedCommand           string        `toml:"seed_command,omitempty" json:"seed_command,omitempty"`
}

type File struct {
	GuestPath  string   `toml:"guest_path,omitempty" json:"guest_path,omitempty" validate:"required"`
	LocalPath  string   `toml:"local_path,omitempty" json:"local_path,omitempty"`
	SecretName string   `toml:"secret_name,omitempty" json:"secret_name,omitempty"`
	RawValue   string   `toml:"raw_value,omitempty" json:"raw_value,omitempty"`
	Processes  []string `json:"processes,omitempty" toml:"processes,omitempty"`
}

func (f File) toMachineFile() (*fly.File, error) {
	file := &fly.File{
		GuestPath: f.GuestPath,
	}
	switch {
	case f.LocalPath != "":
		content, err := os.ReadFile(f.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("could not read file %s: %w", f.LocalPath, err)
		}
		rawValue := base64.StdEncoding.EncodeToString(content)
		file.RawValue = &rawValue
	case f.SecretName != "":
		file.SecretName = &f.SecretName
	case f.RawValue != "":
		encodedValue := base64.StdEncoding.EncodeToString([]byte(f.RawValue))
		file.RawValue = &encodedValue
	}
	return file, nil
}

type Static struct {
	GuestPath     string `toml:"guest_path" json:"guest_path,omitempty" validate:"required"`
	UrlPrefix     string `toml:"url_prefix" json:"url_prefix,omitempty" validate:"required"`
	TigrisBucket  string `toml:"tigris_bucket,omitempty" json:"tigris_bucket"`
	IndexDocument string `toml:"index_document,omitempty" json:"index_document,omitempty"`
}

type Mount struct {
	Source                  string   `toml:"source,omitempty" json:"source,omitempty"`
	Destination             string   `toml:"destination,omitempty" json:"destination,omitempty"`
	InitialSize             string   `toml:"initial_size,omitempty" json:"initial_size,omitempty"`
	SnapshotRetention       *int     `toml:"snapshot_retention,omitempty" json:"snapshot_retention,omitempty"`
	AutoExtendSizeThreshold int      `toml:"auto_extend_size_threshold,omitempty" json:"auto_extend_size_threshold,omitempty"`
	AutoExtendSizeIncrement string   `toml:"auto_extend_size_increment,omitempty" json:"auto_extend_size_increment,omitempty"`
	AutoExtendSizeLimit     string   `toml:"auto_extend_size_limit,omitempty" json:"auto_extend_size_limit,omitempty"`
	Processes               []string `toml:"processes,omitempty" json:"processes,omitempty"`
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
	Cmd            []string `toml:"cmd,omitempty" json:"cmd,omitempty"`
	Entrypoint     []string `toml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Exec           []string `toml:"exec,omitempty" json:"exec,omitempty"`
	AutoRollback   bool     `toml:"auto_rollback,omitempty" json:"auto_rollback,omitempty"`
	EnableConsul   bool     `toml:"enable_consul,omitempty" json:"enable_consul,omitempty"`
	EnableEtcd     bool     `toml:"enable_etcd,omitempty" json:"enable_etcd,omitempty"`
	LazyLoadImages bool     `toml:"lazy_load_images,omitempty" json:"lazy_load_images,omitempty"`
	Attached       Attached `toml:"attached,omitempty" json:"attached,omitempty"`
	MachineConfig  string   `toml:"machine_config,omitempty" json:"machine_config,omitempty"`
	UseZstd        bool     `toml:"use_zstd,omitempty" json:"use_zstd,omitempty"`
}

type Attached struct {
	Secrets AttachedSecrets `toml:"secrets,omitempty" json:"secrets,omitempty"`
}

type AttachedSecrets struct {
	Export map[string]string `toml:"export,omitempty" json:"export,omitempty"`
}

type Compute struct {
	Size              string `json:"size,omitempty" toml:"size,omitempty"`
	Memory            string `json:"memory,omitempty" toml:"memory,omitempty"`
	*fly.MachineGuest `toml:",inline" json:",inline"`
	Processes         []string `json:"processes,omitempty" toml:"processes,omitempty"`
}
type Restart struct {
	Policy     RestartPolicy `toml:"policy,omitempty" json:"policy,omitempty"`
	MaxRetries int           `toml:"retries,omitempty" json:"retries,omitempty"`
	Processes  []string      `json:"processes,omitempty" toml:"processes,omitempty"`
}

func (c *Config) ConfigFilePath() string {
	return c.configFilePath
}

func (c *Config) SetConfigFilePath(configFilePath string) {
	c.configFilePath = configFilePath
}

func (c *Config) DetermineIPType(ipType string) string {
	// If the app is a flycast app, then it requires a private IP
	if ipType == "private" {
		return "private"
	}

	// If there is a service that is not http or https on standard points, then it requires a dedicated IP
	for _, service := range c.Services {
		switch service.Protocol {
		case "udp":
			return "dedicated"
		case "tcp":
			for _, p := range service.Ports {
				if p.HasNonHttpPorts() {
					return "dedicated"
				} else if p.ContainsPort(80) && !reflect.DeepEqual(p.Handlers, []string{"http"}) {
					return "dedicated"
				} else if p.ContainsPort(443) && !(reflect.DeepEqual(p.Handlers, []string{"http", "tls"}) || reflect.DeepEqual(p.Handlers, []string{"tls", "http"})) {
					return "dedicated"
				}
			}
		}
	}

	// Use shared IP if there are no services that require a dedicated IP
	return "shared"
}

// IsUsingGPU returns true if any VMs have a gpu-kind set.
func (c *Config) IsUsingGPU() bool {
	for _, vm := range c.Compute {
		if vm != nil && vm.MachineGuest != nil && vm.MachineGuest.GPUKind != "" {
			return true
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
	if c == nil || c.Build == nil {
		return ""
	}
	return c.Build.Dockerfile
}

func (c *Config) Ignorefile() string {
	if c == nil || c.Build == nil {
		return ""
	}
	return c.Build.Ignorefile
}

func (c *Config) DockerBuildTarget() string {
	if c == nil || c.Build == nil {
		return ""
	}
	return c.Build.DockerBuildTarget
}

func (c *Config) InternalPort() int {
	if c.HTTPService != nil {
		return c.HTTPService.InternalPort
	}

	if len(c.Services) > 0 {
		return c.Services[0].InternalPort
	}
	return 0
}

func (cfg *Config) BuildStrategies() []string {
	strategies := []string{}

	if cfg == nil || cfg.Build == nil {
		return strategies
	}

	if cfg.Build.Image != "" {
		strategies = append(strategies, fmt.Sprintf("the \"%s\" docker image", cfg.Build.Image))
	}
	if cfg.Build.Builder != "" || len(cfg.Build.Buildpacks) > 0 {
		strategies = append(strategies, "a buildpack")
	}
	if cfg.Build.Dockerfile != "" || cfg.Build.DockerBuildTarget != "" {
		if cfg.Build.Dockerfile != "" {
			strategies = append(strategies, fmt.Sprintf("the \"%s\" dockerfile", cfg.Build.Dockerfile))
		} else {
			strategies = append(strategies, "a dockerfile")
		}
	}
	if cfg.Build.Builtin != "" {
		strategies = append(strategies, fmt.Sprintf("the \"%s\" builtin image", cfg.Build.Builtin))
	}

	return strategies
}

func (cfg *Config) URL() *url.URL {
	u := &url.URL{
		Scheme: "https",
		Host:   cfg.AppName + ".fly.dev",
		Path:   "/",
	}

	// HTTPService always listen on https, even if ForceHTTPS is false
	if cfg.HTTPService != nil && cfg.HTTPService.InternalPort > 0 {
		return u
	}

	var httpPorts []int
	var httpsPorts []int
	for _, service := range cfg.Services {
		for _, port := range service.Ports {
			if port.Port == nil || !slices.Contains(port.Handlers, "http") {
				continue
			}
			if slices.Contains(port.Handlers, "tls") {
				httpsPorts = append(httpsPorts, *port.Port)
			} else {
				httpPorts = append(httpPorts, *port.Port)
			}
		}
	}

	switch {
	case slices.Contains(httpsPorts, 443):
		return u
	case slices.Contains(httpPorts, 80):
		u.Scheme = "http"
		return u
	case len(httpsPorts) > 0:
		slices.Sort(httpsPorts)
		u.Host = fmt.Sprintf("%s:%d", u.Host, httpsPorts[0])
		return u
	case len(httpPorts) > 0:
		slices.Sort(httpPorts)
		u.Host = fmt.Sprintf("%s:%d", u.Host, httpPorts[0])
		u.Scheme = "http"
		return u
	default:
		return nil
	}
}

// MergeFiles merges the provided files with the files in the config wherein the provided files
// take precedence.
func (cfg *Config) MergeFiles(files []*fly.File) error {
	// First convert the Config files to Machine files.
	cfgFiles := make([]*fly.File, 0, len(cfg.Files))
	for _, f := range cfg.Files {
		machineFile, err := f.toMachineFile()
		if err != nil {
			return err
		}
		cfgFiles = append(cfgFiles, machineFile)
	}

	// Merge the config files with the provided files.
	mConfig := &fly.MachineConfig{
		Files: cfgFiles,
	}
	fly.MergeFiles(mConfig, files)

	// Persist the merged files back to the config to be used later for deploying.
	cfg.MergedFiles = mConfig.Files

	return nil
}

func (cfg *Config) DeployStrategy() string {
	if cfg.Deploy == nil {
		return ""
	}
	return cfg.Deploy.Strategy
}
