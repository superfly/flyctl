package appv2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
)

// LoadConfig loads the app config at the given path.
func LoadConfig(path string) (cfg *Config, err error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg, err = unmarshalTOML(buf)
	if err != nil {
		return nil, err
	}

	cfg.configFilePath = path
	// cfg.WriteToFile("patched-fly.toml")
	return cfg, nil
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

	err = c.marshalTOML(file)
	return
}

func (c *Config) WriteToDisk(ctx context.Context, path string) (err error) {
	io := iostreams.FromContext(ctx)
	err = c.WriteToFile(path)
	fmt.Fprintf(io.Out, "Wrote config file %s\n", helpers.PathRelativeToCWD(path))
	return
}

func unmarshalTOML(buf []byte) (*Config, error) {
	// Keep this map as vanilla as possible
	// This is what we send to Web API for Nomad apps
	rawDefinition := map[string]any{}
	if err := toml.Unmarshal(buf, &rawDefinition); err != nil {
		return nil, err
	}

	// Unmarshal twice due to in-place updates
	cfgMap := map[string]any{}
	if err := toml.Unmarshal(buf, &cfgMap); err != nil {
		return nil, err
	}

	cfg, err := applyPatches(cfgMap)
	// In case of parsing error fallback to Nomad only compatibility
	if err != nil {
		cfg = &Config{parseError: err}
		if name, ok := (rawDefinition["app"]).(string); ok {
			cfg.AppName = name
		}
		cfg.Build = unmarshalBuild(rawDefinition)
	}

	cfg.RawDefinition = rawDefinition
	return cfg, nil
}

func (c *Config) marshalTOML(w io.Writer) error {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}

	fmt.Fprintf(w, "# fly.toml file generated for %s on %s\n\n", c.AppName, time.Now().Format(time.RFC3339))
	_, err := b.WriteTo(w)
	return err
}

func (c *Config) toTOMLString() (string, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return "", err
	} else {
		return b.String(), nil
	}
}

// Fallback method when we fail to parse fly.toml into Config
// XXX: High chances we can ditch and unmarshal directly into Build struct
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
		case "ignorefile":
			b.Ignorefile = fmt.Sprint(v)
			configValueSet = configValueSet || b.Ignorefile != ""
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
