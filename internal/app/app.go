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
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
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
func LoadConfig(ctx context.Context, path string, platformVersion string) (cfg *Config, err error) {
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
	cfg.platformVersion = platformVersion

	if platformVersion == "" {
		cfg.DeterminePlatform(ctx, file)
	}

	err = cfg.unmarshalTOML(file)

	return
}

// Use this type to unmarshal fly.toml with the goal of retreiving the app name only
type SlimConfig struct {
	AppName string `toml:"app,omitempty"`
}

// Config wraps the properties of app configuration.
type Config struct {
	AppName         string                      `toml:"app,omitempty"`
	Build           *Build                      `toml:"build,omitempty"`
	HttpService     *HttpService                `toml:"http_service,omitempty"`
	Definition      map[string]interface{}      `toml:"definition,omitempty"`
	Path            string                      `toml:"path,omitempty"`
	Services        []api.MachineService        `toml:"services"`
	Env             map[string]string           `toml:"env" json:"env"`
	Metrics         *api.MachineMetrics         `toml:"metrics" json:"metrics"`
	Statics         []*Static                   `toml:"statics,omitempty" json:"statics"`
	Deploy          *Deploy                     `toml:"deploy, omitempty"`
	PrimaryRegion   string                      `toml:"primary_region,omitempty"`
	Checks          map[string]api.MachineCheck `toml:"checks,omitempty"`
	platformVersion string
}

type Deploy struct {
	ReleaseCommand string `toml:"release_command,omitempty"`
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix" validate:"required"`
}
type HttpService struct {
	InternalPort int                            `json:"internal_port" toml:"internal_port" validate:"required,numeric"`
	ForceHttps   bool                           `toml:"force_https"`
	Concurrency  *api.MachineServiceConcurrency `toml:"concurrency,omitempty"`
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
	DockerBuildTarget string                 `toml:"buildpacks,omitempty"`
}

// SetMachinesPlatform informs the TOML marshaller that this config is for the machines platform
func (c *Config) SetMachinesPlatform() {
	c.platformVersion = MachinesPlatform
}

// SetNomadPlatform informs the TOML marshaller that this config is for the nomad platform
func (c *Config) SetNomadPlatform() {
	c.platformVersion = NomadPlatform
}

func (c *Config) SetPlatformVersion(platform string) {
	c.platformVersion = platform
}

// ForMachines is true when the config is intended for the machines platform
func (c *Config) ForMachines() bool {
	return c.platformVersion == MachinesPlatform
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

func (c *Config) DeterminePlatform(ctx context.Context, r io.ReadSeeker) (err error) {
	client := client.FromContext(ctx)
	slimConfig := &SlimConfig{}
	_, err = toml.NewDecoder(r).Decode(&slimConfig)

	if err != nil {
		return err
	}

	basicApp, err := client.API().GetAppBasic(ctx, slimConfig.AppName)
	if err != nil {
		return err
	}

	if basicApp.PlatformVersion == MachinesPlatform {
		c.SetMachinesPlatform()
	} else {
		c.SetNomadPlatform()
	}

	return
}

func (c *Config) unmarshalTOML(r io.ReadSeeker) (err error) {
	var data map[string]interface{}

	slimConfig := &SlimConfig{}

	// Fetch the app name only, to check which platform we're on via the API
	if _, err = toml.NewDecoder(r).Decode(&slimConfig); err == nil {

		if err != nil {
			return err
		}

		// Rewind TOML in preparation for parsing the full config
		r.Seek(0, io.SeekStart)
		if c.ForMachines() {
			_, err = toml.NewDecoder(r).Decode(&c)

			if err != nil {
				return err
			}

		} else {
			_, err = toml.NewDecoder(r).Decode(&data)

			if err != nil {
				return err
			}

			err = c.unmarshalNativeMap(data)
		}
	}

	return
}

func (c *Config) unmarshalNativeMap(data map[string]interface{}) error {
	if name, ok := (data["app"]).(string); ok {
		c.AppName = name
	}
	delete(data, "app")

	c.Build = unmarshalBuild(data)
	delete(data, "build")

	for k := range c.Definition {
		delete(c.Definition, k)
	}
	c.Definition = data

	return nil
}

func unmarshalBuild(data map[string]interface{}) *Build {
	buildConfig, ok := (data["build"]).(map[string]interface{})
	if !ok {
		return nil
	}

	b := &Build{
		Args:       map[string]string{},
		Settings:   map[string]interface{}{},
		Buildpacks: []string{},
	}

	configValueSet := false
	for k, v := range buildConfig {
		switch k {
		case "builder":
			b.Builder = fmt.Sprint(v)
			configValueSet = configValueSet || b.Builder != ""
		case "buildpacks":
			if bpSlice, ok := v.([]interface{}); ok {
				for _, argV := range bpSlice {
					b.Buildpacks = append(b.Buildpacks, fmt.Sprint(argV))
				}
			}
		case "args":
			if argMap, ok := v.(map[string]interface{}); ok {
				for argK, argV := range argMap {
					b.Args[argK] = fmt.Sprint(argV)
				}
			}
		case "builtin":
			b.Builtin = fmt.Sprint(v)
			configValueSet = configValueSet || b.Builtin != ""
		case "settings":
			if settingsMap, ok := v.(map[string]interface{}); ok {
				for settingK, settingV := range settingsMap {
					b.Settings[settingK] = settingV // fmt.Sprint(argV)
				}
			}
		case "image":
			b.Image = fmt.Sprint(v)
			configValueSet = configValueSet || b.Image != ""
		case "dockerfile":
			b.Dockerfile = fmt.Sprint(v)
			configValueSet = configValueSet || b.Dockerfile != ""
		case "build_target", "build-target":
			b.DockerBuildTarget = fmt.Sprint(v)
			configValueSet = configValueSet || b.DockerBuildTarget != ""
		default:
			b.Args[k] = fmt.Sprint(v)
		}
	}

	if !configValueSet && len(b.Args) == 0 {
		return nil
	}

	return b
}

func (c *Config) marshalTOML(w io.Writer) error {
	var b bytes.Buffer

	encoder := toml.NewEncoder(&b)
	fmt.Fprintf(w, "# fly.toml file generated for %s on %s\n\n", c.AppName, time.Now().Format(time.RFC3339))

	// For machines apps, encode and write directly, bypassing custom marshalling
	if c.platformVersion == MachinesPlatform {
		encoder.Encode(&c)
		_, err := b.WriteTo(w)
		return err
	}

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

func (c *Config) SetConcurrency(soft int, hard int) bool {
	services, ok := c.Definition["services"].([]interface{})
	if !ok {
		return false
	}

	if len(services) == 0 {
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
