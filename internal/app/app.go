// Package app implements functionality related to reading and writing app
// configuration files.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/sourcecode"
)

// DefaultConfigFileName denotes the default application configuration file name.
const DefaultConfigFileName = "fly.toml"

// LoadConfig loads the app config at the given path.
func LoadConfig(path string) (cfg *Config, err error) {
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

// Config wraps the properties of app configuration.
type Config struct {
	AppName    string
	Build      *Build
	Definition map[string]interface{}
	Path       string
}

type Build struct {
	Builder    string
	Args       map[string]string
	Buildpacks []string
	// Or...
	Builtin  string
	Settings map[string]interface{}
	// Or...
	Image string
	// Or...
	Dockerfile        string
	DockerBuildTarget string
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

func (c *Config) DockerBuildTarget() string {
	if c.Build == nil {
		return ""
	}
	return c.Build.DockerBuildTarget
}

func (c *Config) EncodeTo(w io.Writer) error {
	return c.marshalTOML(w)
}

func (c *Config) unmarshalTOML(r io.Reader) (err error) {
	var data map[string]interface{}

	if _, err = toml.DecodeReader(r, &data); err == nil {
		err = c.unmarshalNativeMap(data)
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

	for k, v := range buildConfig {
		switch k {
		case "builder":
			b.Builder = fmt.Sprint(v)
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
		case "settings":
			if settingsMap, ok := v.(map[string]interface{}); ok {
				for settingK, settingV := range settingsMap {
					b.Settings[settingK] = settingV //fmt.Sprint(argV)
				}
			}
		case "image":
			b.Image = fmt.Sprint(v)
		case "dockerfile":
			b.Dockerfile = fmt.Sprint(v)
		case "build_target":
			b.DockerBuildTarget = fmt.Sprint(v)
		default:
			b.Args[k] = fmt.Sprint(v)
		}
	}

	if b.Builder == "" && b.Builtin == "" && b.Image == "" && b.Dockerfile == "" && len(b.Args) == 0 {
		return nil
	}

	return b
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

func (c *Config) SetEnvVariables(vals map[string]string) {
	env := c.GetEnvVariables()

	for k, v := range vals {
		env[k] = v
	}

	c.Definition["env"] = env
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
	env := c.GetEnvVariables()

	env[name] = value

	c.Definition["env"] = env
}

func (c *Config) GetEnvVariables() map[string]string {
	env := map[string]string{}

	if rawEnv, ok := c.Definition["env"]; ok {
		if castEnv, ok := rawEnv.(map[string]interface{}); ok {
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

func (c *Config) SetStatics(statics []sourcecode.Static) {
	c.Definition["statics"] = statics
}

func (c *Config) SetVolumes(volumes []sourcecode.Volume) {
	c.Definition["mounts"] = volumes
}
