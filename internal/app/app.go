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
	"golang.org/x/exp/slices"
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
	cfg = NewConfig()

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

func (svc *HTTPService) toMachineService() *api.MachineService {
	concurrency := svc.Concurrency
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
		InternalPort: svc.InternalPort,
		Ports: []api.MachinePort{{
			Port:       api.IntPointer(80),
			Handlers:   []string{"http"},
			ForceHttps: svc.ForceHttps,
		}, {
			Port:     api.IntPointer(443),
			Handlers: []string{"http", "tls"},
		}},
		Concurrency: concurrency,
	}
}

func (svc *Service) toMachineService() *api.MachineService {
	checks := make([]api.MachineCheck, 0, len(svc.TCPChecks)+len(svc.HTTPChecks))
	for _, tc := range svc.TCPChecks {
		checks = append(checks, *tc.toMachineCheck())
	}
	for _, hc := range svc.HTTPChecks {
		checks = append(checks, *hc.toMachineCheck())
	}
	return &api.MachineService{
		Protocol:     svc.Protocol,
		InternalPort: svc.InternalPort,
		Ports:        svc.Ports,
		Concurrency:  svc.Concurrency,
		Checks:       checks,
	}
}

type ServiceTCPCheck struct {
	Interval *api.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout  *api.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval          *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

type ToplevelCheck struct {
	Port              *int              `json:"port,omitempty" toml:"port,omitempty"`
	Type              *string           `json:"type,omitempty" toml:"type,omitempty"`
	Interval          *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

func (chk *ToplevelCheck) toMachineCheck() (*api.MachineCheck, error) {
	if chk.Type == nil || !slices.Contains([]string{"http", "tcp"}, *chk.Type) {
		return nil, fmt.Errorf("Missing or invalid check type, must be 'http' or 'tcp'")
	}

	res := &api.MachineCheck{
		Type:              chk.Type,
		Port:              chk.Port,
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPHeaders: lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) api.MachineHTTPHeader {
				return api.MachineHTTPHeader{Name: k, Values: []string{v}}
			}),
	}
	if chk.HTTPMethod != nil {
		res.HTTPMethod = api.Pointer(strings.ToUpper(*chk.HTTPMethod))
	}
	return res, nil
}

func (chk *ToplevelCheck) String() string {
	chkType := "none"
	if chk.Type != nil {
		chkType = *chk.Type
	}
	switch chkType {
	case "tcp":
		return fmt.Sprintf("tcp-%d", chk.Port)
	case "http":
		return fmt.Sprintf("http-%d-%v", chk.Port, chk.HTTPMethod)
	default:
		return fmt.Sprintf("%s-%d", chkType, chk.Port)
	}
}

func (chk *ServiceHTTPCheck) toMachineCheck() *api.MachineCheck {
	return &api.MachineCheck{
		Type:              api.Pointer("http"),
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		HTTPMethod:        chk.HTTPMethod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPHeaders: lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) api.MachineHTTPHeader {
				return api.MachineHTTPHeader{Name: k, Values: []string{v}}
			}),
	}
}

func (chk *ServiceHTTPCheck) String(port int) string {
	return fmt.Sprintf("http-%d-%v", port, chk.HTTPMethod)
}

func (chk *ServiceTCPCheck) toMachineCheck() *api.MachineCheck {
	return &api.MachineCheck{
		Type:     api.Pointer("tcp"),
		Interval: chk.Interval,
		Timeout:  chk.Timeout,
	}
}

func (chk *ServiceTCPCheck) String(port int) string {
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
	if _, err = toml.NewDecoder(r).Decode(&c); err != nil {
		return err
	}
	c.Definition = definition
	return nil
}

func (c *Config) marshalTOML(w io.Writer) error {
	var b bytes.Buffer
	tomlEncoder := toml.NewEncoder(&b)

	rawData := c.Definition
	rawData["app"] = c.AppName

	if c.Build != nil {
		buildData := make(map[string]interface{})
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

	var err error
	if rawData, err = normalizeDefinition(rawData); err != nil {
		return err
	}
	if err := tomlEncoder.Encode(rawData); err != nil {
		return err
	}

	fmt.Fprintf(w, "# fly.toml file generated for %s on %s\n\n", c.AppName, time.Now().Format(time.RFC3339))
	_, err = b.WriteTo(w)
	return err
}

// normalizeDefinition roundtrips through json encoder to convert
// float64 numbers to json.Number, otherwise numbers are floats in toml
func normalizeDefinition(src map[string]interface{}) (map[string]interface{}, error) {
	if len(src) == 0 {
		return src, nil
	}

	dst := make(map[string]interface{})

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(src); err != nil {
		return nil, err
	}

	d := json.NewDecoder(&buf)
	d.UseNumber()
	if err := d.Decode(&dst); err != nil {
		return nil, err
	}

	return dst, nil
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

func (c *Config) SetConcurrency(soft int, hard int) bool {
	services, ok := c.Definition["services"].([]interface{})
	if !ok || len(services) == 0 {
		return false
	}

	if service, ok := services[0].(map[string]interface{}); ok {
		if concurrency, ok := service["concurrency"].(map[string]interface{}); ok {
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

func (c *Config) GetProcessConfigs(appLaunching bool) (map[string]*ProcessConfig, error) {
	res := make(map[string]*ProcessConfig)
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
		res[processName] = &ProcessConfig{
			Cmd:      cmd,
			Services: make([]api.MachineService, 0),
			Checks:   make(map[string]api.MachineCheck),
		}
	}

	for checkName, check := range c.Checks {
		machineCheck, err := check.toMachineCheck()
		if err != nil {
			return nil, err
		}
		for _, pc := range res {
			pc.Checks[checkName] = *machineCheck
		}
	}

	if c.HttpService != nil {
		if processCount > 1 {
			return nil, fmt.Errorf("http_service is not supported when more than one processes are defined "+
				"for an app, and this app has %d processes", processCount)
		}
		toUpdate := res[defaultProcessName]
		toUpdate.Services = append(toUpdate.Services, *c.HttpService.toMachineService())
	}

	for _, service := range c.Services {
		switch {
		case len(service.Processes) == 0 && processCount > 0:
			return nil, fmt.Errorf("error service has no processes set and app has %d processes defined;"+
				"update fly.toml to set processes for each service", processCount)
		case len(service.Processes) == 0 || processCount == 0:
			processName := defaultProcessName
			pc, present := res[processName]
			if processCount > 0 && !present {
				return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
					"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
			}
			pc.Services = append(pc.Services, *service.toMachineService())
		default:
			for _, processName := range service.Processes {
				pc, present := res[processName]
				if !present {
					return nil, fmt.Errorf("error service specifies '%s' as one of its processes, but no "+
						"processes are defined with that name; update fly.toml [processes] to include a %s process", processName, processName)
				}
				pc.Services = append(pc.Services, *service.toMachineService())
			}
		}
	}
	return res, nil
}
