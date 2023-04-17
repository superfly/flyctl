package appconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
)

const flytomlHeader = `# fly.toml file generated for %s on %s
#
# See https://fly.io/docs/reference/configuration/ for  information about how to use this file.
#
`

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

func (c *Config) WriteTo(w io.Writer) error {
	b, err := c.marshalTOML()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, flytomlHeader, c.AppName, time.Now().Format(time.RFC3339))
	if err != nil {
		return err
	}
	_, err = bytes.NewBuffer(b).WriteTo(w)
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

	err = c.WriteTo(file)
	return
}

func (c *Config) WriteToDisk(ctx context.Context, path string) (err error) {
	io := iostreams.FromContext(ctx)
	err = c.WriteToFile(path)
	fmt.Fprintf(io.Out, "Wrote config file %s\n", helpers.PathRelativeToCWD(path))
	return
}

// MarshalJSON implements the json.Marshaler interface
func (c *Config) MarshalJSON() ([]byte, error) {
	switch {
	case c == nil:
		return json.Marshal(nil)
	case c.platformVersion == MachinesPlatform:
		return json.Marshal(*c)
	default:
		sections, err := c.rawSections()
		if err != nil {
			return nil, err
		}
		return json.Marshal(lo.Assign(sections...))
	}
}

// marshalTOML serializes the configuration to TOML format
// NOTES:
//   * It can't be called `MarshalTOML` because toml libraries don't support marshaler interface on root values
//   * Needs to reimplements most of MarshalJSON to enforce order of fields
//   * Instead of this, you usually need one WriteTo(), WriteToFile() or WriteToDisk()
//
func (c *Config) marshalTOML() ([]byte, error) {
	var b bytes.Buffer
	encoder := toml.NewEncoder(&b)

	switch {
	case c == nil:
		break
	case c.platformVersion == MachinesPlatform:
		if err := encoder.Encode(c); err != nil {
			return nil, err
		}
	default:
		// FallBack for Nomad apps
		sections, err := c.rawSections()
		if err != nil {
			return nil, err
		}
		for _, section := range sections {
			if err := encoder.Encode(section); err != nil {
				return nil, err
			}
		}
	}
	return b.Bytes(), nil
}

// rawSections returns configuration parts in serialization order for Nomad apps
func (c *Config) rawSections() ([]map[string]any, error) {
	// Write app name first to be sure it will be there at the top
	sections := []map[string]any{
		{"app": c.AppName},
	}

	rawData := c.SanitizedDefinition()
	// Restore sections removed by SanitizedDefinition
	if c.Build != nil {
		rawData["build"] = c.Build
	}
	if c.PrimaryRegion != "" {
		rawData["primary_region"] = c.PrimaryRegion
	}
	if c.HTTPService != nil {
		rawData["http_service"] = c.HTTPService
	}

	if len(rawData) > 0 {
		// roundtrip through json encoder to convert float64 numbers to json.Number,
		// otherwise numbers are floats in toml
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(rawData); err != nil {
			return nil, err
		}

		d := json.NewDecoder(&buf)
		d.UseNumber()
		if err := d.Decode(&rawData); err != nil {
			return nil, err
		}
		sections = append(sections, rawData)
	}

	return sections, nil
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
		cfg = &Config{v2UnmarshalError: err}
		if name, ok := (rawDefinition["app"]).(string); ok {
			cfg.AppName = name
		}
		cfg.Build = unmarshalBuild(rawDefinition)
	}

	cfg.RawDefinition = rawDefinition
	return cfg, nil
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
