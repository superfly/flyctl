// Package app implements functionality related to reading and writing app
// configuration files.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"
	"github.com/google/shlex"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
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

func NewConfig() *Config {
	return &Config{
		Definition: map[string]interface{}{},
	}
}

// LoadConfig loads the app config at the given path.
func LoadConfig(ctx context.Context, path string) (cfg *Config, err error) {
	cfg = &Config{
		Definition: map[string]interface{}{},
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := file.Close(); err == nil {
			err = e
		}
	}()

	cfg.Path = path
	err = cfg.unmarshalTOML(file)
	return
}

// Use this type to unmarshal fly.toml with the goal of retreiving the app name only
type SlimConfig struct {
	AppName string `toml:"app,omitempty"`
}

// Config wraps the properties of app configuration.
type Config struct {
	AppName       string                    `toml:"app,omitempty" json:"app,omitempty"`
	Build         *Build                    `toml:"build,omitempty" json:"build,omitempty"`
	HttpService   *HTTPService              `toml:"http_service,omitempty" json:"http_service,omitempty"`
	Definition    map[string]interface{}    `toml:"definition,omitempty" json:"definition,omitempty"`
	Path          string                    `toml:"path,omitempty" json:"path,omitempty"`
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
}

type Deploy struct {
	ReleaseCommand string `toml:"release_command,omitempty"`
	Strategy       string `toml:"strategy,omitempty"`
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix" validate:"required"`
}
type HTTPService struct {
	InternalPort int                            `json:"internal_port" toml:"internal_port" validate:"required,numeric"`
	ForceHttps   bool                           `toml:"force_https"`
	Concurrency  *api.MachineServiceConcurrency `toml:"concurrency,omitempty"`
}

type Service struct {
	Protocol     string                         `json:"protocol" toml:"protocol"`
	InternalPort int                            `json:"internal_port" toml:"internal_port"`
	Ports        []api.MachinePort              `json:"ports" toml:"ports"`
	Concurrency  *api.MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	TCPChecks    []*ServiceTCPCheck             `json:"tcp_checks,omitempty" toml:"tcp_checks,omitempty"`
	HTTPChecks   []*ServiceHTTPCheck            `json:"http_checks,omitempty" toml:"http_checks,omitempty"`
	Processes    []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
}

func (hs *HTTPService) toMachineService() *api.MachineService {
	concurrency := hs.Concurrency
	if concurrency != nil {
		if concurrency.Type == "" {
			concurrency.Type = "requests"
		}
		if concurrency.HardLimit == 0 {
			concurrency.HardLimit = 25
		}
		if concurrency.SoftLimit == 0 {
			concurrency.SoftLimit = int(math.Ceil(float64(concurrency.HardLimit) * 0.8))
		}
	}
	return &api.MachineService{
		Protocol:     "tcp",
		InternalPort: hs.InternalPort,
		Ports: []api.MachinePort{{
			Port:       api.IntPointer(80),
			Handlers:   []string{"http"},
			ForceHttps: hs.ForceHttps,
		}, {
			Port:     api.IntPointer(443),
			Handlers: []string{"http", "tls"},
		}},
		Concurrency: concurrency,
	}
}

func (s *Service) toMachineService() *api.MachineService {
	checks := make([]api.Check, 0, len(s.TCPChecks)+len(s.HTTPChecks))
	for _, tc := range s.TCPChecks {
		checks = append(checks, *tc.toCheck())
	}
	for _, hc := range s.HTTPChecks {
		checks = append(checks, *hc.toCheck())
	}
	return &api.MachineService{
		Protocol:     s.Protocol,
		InternalPort: s.InternalPort,
		Ports:        s.Ports,
		Concurrency:  s.Concurrency,
		Checks:       checks,
	}
}

type ServiceTCPCheck struct {
	Interval     *api.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout      *api.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod  *api.Duration `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	RestartLimit int           `json:"restart_limit,omitempty" toml:"restart_limit,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval      *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout       *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod   *api.Duration     `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	RestartLimit  int               `json:"restart_limit,omitempty" toml:"restart_limit,omitempty"`
	HTTPMethod    string            `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath      string            `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol  string            `json:"protocol,omitempty" toml:"protocol,omitempty"`
	TLSSkipVerify bool              `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	Headers       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

type ToplevelCheck struct {
	Type          string            `json:"type,omitempty" toml:"type,omitempty"`
	Port          int               `json:"port,omitempty" toml:"port,omitempty"`
	Interval      *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout       *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod   *api.Duration     `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	RestartLimit  int               `json:"restart_limit,omitempty" toml:"restart_limit,omitempty"`
	HTTPMethod    string            `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath      string            `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol  string            `json:"protocol,omitempty" toml:"protocol,omitempty"`
	TLSSkipVerify bool              `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	Headers       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

func (c *ToplevelCheck) toMachineCheck(launching bool) (*api.MachineCheck, error) {
	// don't error when launching; it's a bad experience!
	if !launching && c.GracePeriod != nil {
		return nil, fmt.Errorf("checks for machines do not yet support grace_period")
	}
	if c.RestartLimit != 0 {
		return nil, fmt.Errorf("checks for machines do not yet support restart_limit")
	}
	if c.HTTPProtocol != "" {
		return nil, fmt.Errorf("checks for machines do not yet support protocol")
	}
	if len(c.Headers) > 0 {
		return nil, fmt.Errorf("checks for machines do not yet support headers")
	}
	res := &api.MachineCheck{
		Type:     c.Type,
		Port:     uint16(c.Port),
		Interval: c.Interval,
		Timeout:  c.Timeout,
	}
	switch c.Type {
	case "tcp":
	case "http":
		methodUpper := strings.ToUpper(c.HTTPMethod)
		res.HTTPMethod = &methodUpper
		res.HTTPPath = &c.HTTPPath
		// FIXME: enable this when machines support it
		// res.TLSSkipVerify = &c.TLSSkipVerify
	default:
		return nil, fmt.Errorf("error unknown check type: %s", c.Type)
	}
	return res, nil
}

func (c *ToplevelCheck) String() string {
	switch c.Type {
	case "tcp":
		return fmt.Sprintf("tcp-%d", c.Port)
	case "http":
		return fmt.Sprintf("http-%d-%s", c.Port, c.HTTPMethod)
	default:
		return fmt.Sprintf("%s-%d", c.Type, c.Port)
	}
}

func (hc *ServiceHTTPCheck) toCheck() *api.Check {
	check := &api.Check{Type: "http"}
	if hc.Interval != nil {
		check.Interval = api.Pointer(uint64(hc.Interval.Milliseconds()))
	}
	if hc.Timeout != nil {
		check.Timeout = api.Pointer(uint64(hc.Timeout.Milliseconds()))
	}
	if hc.GracePeriod != nil {
		check.GracePeriod = api.Pointer(uint64(hc.GracePeriod.Milliseconds()))
	}
	if hc.RestartLimit > 0 {
		check.RestartLimit = api.Pointer(uint64(hc.RestartLimit))
	}

	if hc.HTTPMethod != "" {
		check.HTTPMethod = api.Pointer(hc.HTTPMethod)
	}
	if hc.HTTPPath != "" {
		check.HTTPPath = api.Pointer(hc.HTTPPath)
	}
	if hc.HTTPProtocol != "" {
		check.HTTPProtocol = api.Pointer(hc.HTTPProtocol)
	}
	check.HTTPSkipTLSVerify = api.Pointer(hc.TLSSkipVerify)
	if len(hc.Headers) > 0 {
		check.HTTPHeaders = make([]api.HTTPHeader, 0, len(hc.Headers))
		for name, value := range hc.Headers {
			check.HTTPHeaders = append(check.HTTPHeaders, api.HTTPHeader{Name: name, Value: value})
		}
	}
	return check
}

func (hc *ServiceHTTPCheck) String(port int) string {
	return fmt.Sprintf("http-%d-%s", port, hc.HTTPMethod)
}

func (tc *ServiceTCPCheck) toCheck() *api.Check {
	check := &api.Check{Type: "tcp"}
	if tc.Interval != nil {
		check.Interval = api.Pointer(uint64(tc.Interval.Milliseconds()))
	}
	if tc.Timeout != nil {
		check.Timeout = api.Pointer(uint64(tc.Timeout.Milliseconds()))
	}
	if tc.GracePeriod != nil {
		check.GracePeriod = api.Pointer(uint64(tc.GracePeriod.Milliseconds()))
	}
	if tc.RestartLimit > 0 {
		check.RestartLimit = api.Pointer(uint64(tc.RestartLimit))
	}
	return check
}

func (tc *ServiceTCPCheck) String(port int) string {
	return fmt.Sprintf("tcp-%d", port)
}

type VM struct {
	CpuCount int `toml:"cpu_count,omitempty"`
	Memory   int `toml:"memory,omitempty"`
}

type Build struct {
	Builder           string                 `toml:"builder,omitempty"`
	Args              map[string]string      `toml:"args,omitempty"`
	Buildpacks        []string               `toml:"buildpacks,omitempty"`
	Image             string                 `toml:"image,omitempty"`
	Settings          map[string]interface{} `toml:"settings,omitempty"`
	Builtin           string                 `toml:"builtin,omitempty"`
	Dockerfile        string                 `toml:"dockerfile,omitempty"`
	Ignorefile        string                 `toml:"ignorefile,omitempty"`
	DockerBuildTarget string                 `toml:"build-target,omitempty"`
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

func (ac *Config) HasBuilder() bool {
	return ac.Build != nil && ac.Build.Builder != ""
}

func (ac *Config) HasBuiltin() bool {
	return ac.Build != nil && ac.Build.Builtin != ""
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

func (ac *Config) Image() string {
	if ac.Build == nil {
		return ""
	}
	return ac.Build.Image
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

func (c *Config) EncodeTo(w io.Writer) error {
	return c.marshalTOML(w)
}

func (c *Config) unmarshalTOML(r io.ReadSeeker) error {
	var definition map[string]interface{}
	_, err := toml.NewDecoder(r).Decode(&definition)
	if err != nil {
		return err
	}
	delete(definition, "app")
	delete(definition, "build")
	// FIXME: i need to better understand what Definition is being used for
	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	_, err = toml.NewDecoder(r).Decode(&c)
	if err != nil {
		return err
	}
	c.Definition = definition
	return nil
}

func (c *Config) marshalTOML(w io.Writer) error {
	var b bytes.Buffer

	encoder := toml.NewEncoder(&b)
	fmt.Fprintf(w, "# fly.toml file generated for %s on %s\n\n", c.AppName, time.Now().Format(time.RFC3339))

	rawData := map[string]interface{}{
		"app": c.AppName,
	}

	if err := encoder.Encode(rawData); err != nil {
		return err
	}

	rawData = c.Definition

	if c.Build != nil {
		buildData := map[string]interface{}{}
		if c.Build.Builder != "" {
			buildData["builder"] = c.Build.Builder
		}
		if len(c.Build.Buildpacks) > 0 {
			buildData["buildpacks"] = c.Build.Buildpacks
		}
		if len(c.Build.Args) > 0 {
			buildData["args"] = c.Build.Args
		}
		if c.Build.Builtin != "" {
			buildData["builtin"] = c.Build.Builtin
			if len(c.Build.Settings) > 0 {
				buildData["settings"] = c.Build.Settings
			}
		}
		if c.Build.Image != "" {
			buildData["image"] = c.Build.Image
		}
		if c.Build.Dockerfile != "" {
			buildData["dockerfile"] = c.Build.Dockerfile
		}
		rawData["build"] = buildData
	}

	if len(c.Definition) > 0 {
		// roundtrip through json encoder to convert float64 numbers to json.Number, otherwise numbers are floats in toml
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(c.Definition); err != nil {
			return err
		}

		d := json.NewDecoder(&buf)
		d.UseNumber()
		if err := d.Decode(&c.Definition); err != nil {
			return err
		}

		if err := encoder.Encode(c.Definition); err != nil {
			return err
		}
	}

	_, err := b.WriteTo(w)
	return err
}

func (c *Config) WriteToFile(filename string) (err error) {
	if err = helpers.MkdirAll(filename); err != nil {
		return
	}

	var file *os.File
	if file, err = os.Create(filename); err != nil {
		return
	}
	defer func() {
		if e := file.Close(); err == nil {
			err = e
		}
	}()

	err = c.EncodeTo(file)

	return
}

func (c *Config) WriteToDisk(ctx context.Context, path string) (err error) {
	io := iostreams.FromContext(ctx)
	err = c.WriteToFile(path)
	fmt.Fprintf(io.Out, "Wrote config file %s\n", helpers.PathRelativeToCWD(path))
	return
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

// HasServices - Does this config have a services section
func (c *Config) HasServices() bool {
	_, ok := c.Definition["services"].([]interface{})

	return ok
}

func (c *Config) SetInternalPort(port int) bool {
	services, ok := c.Definition["services"].([]interface{})
	if !ok {
		return false
	}

	if len(services) == 0 {
		return false
	}

	if service, ok := services[0].(map[string]interface{}); ok {
		service["internal_port"] = port

		return true
	}

	return false
}

func (c *Config) InternalPort() (int, error) {
	tmpservices, ok := c.Definition["services"]
	if !ok {
		return -1, errors.New("could not find internal port setting")
	}

	services, ok := tmpservices.([]map[string]interface{})
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
		// we get map[string]interface{} when unmarshaling toml, and map[string]string from SetEnvVariables. Support them both :vomit:
		switch castEnv := rawEnv.(type) {
		case map[string]string:
			env = castEnv
		case map[string]interface{}:
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

type ProcessConfig struct {
	Cmd      []string
	Services []api.MachineService
	Checks   map[string]api.MachineCheck
}

func (c *Config) GetProcessConfigs(appLaunching bool) (map[string]ProcessConfig, error) {
	res := make(map[string]ProcessConfig)
	processCount := len(c.Processes)
	configProcesses := lo.Assign(c.Processes)
	if processCount == 0 {
		configProcesses[""] = ""
	}
	defaultProcessName := lo.Keys(configProcesses)[0]

	for processName, cmdStr := range configProcesses {
		cmd := make([]string, 0)
		if cmdStr != "" {
			var err error
			cmd, err = shlex.Split(cmdStr)
			if err != nil {
				return nil, fmt.Errorf("could not parse command for %s process group: %w", processName, err)
			}
		}
		res[processName] = ProcessConfig{
			Cmd:      cmd,
			Services: make([]api.MachineService, 0),
			Checks:   make(map[string]api.MachineCheck),
		}
	}

	for checkName, check := range c.Checks {
		fullCheckName := fmt.Sprintf("chk-%s-%s", checkName, check.String())
		machineCheck, err := check.toMachineCheck(appLaunching)
		if err != nil {
			return nil, err
		}
		for processName := range res {
			procToUpdate := res[processName]
			procToUpdate.Checks[fullCheckName] = *machineCheck
			res[processName] = procToUpdate
		}
	}

	if c.HttpService != nil {
		if processCount > 1 {
			return nil, fmt.Errorf("http_service is not supported when more than one processes are defined "+
				"for an app, and this app has %d processes", processCount)
		}
		servicesToUpdate := res[defaultProcessName]
		servicesToUpdate.Services = append(servicesToUpdate.Services, *c.HttpService.toMachineService())
		res[defaultProcessName] = servicesToUpdate
	}

	for _, service := range c.Services {
		switch {
		case len(service.Processes) == 0 && processCount > 0:
			return nil, fmt.Errorf("error service has no processes set and app has %d processes defined;"+
				"update fly.toml to set processes for each service", processCount)
		case len(service.Processes) == 0 || processCount == 0:
			processName := defaultProcessName
			procConfigToUpdate, present := res[processName]
			if processCount > 0 && !present {
				return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
					"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
			}
			procConfigToUpdate.Services = append(procConfigToUpdate.Services, *service.toMachineService())
			res[processName] = procConfigToUpdate
		default:
			for _, processName := range service.Processes {
				procConfigToUpdate, present := res[processName]
				if !present {
					return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
						"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
				}
				procConfigToUpdate.Services = append(procConfigToUpdate.Services, *service.toMachineService())
				res[processName] = procConfigToUpdate
			}
		}
	}
	return res, nil
}
