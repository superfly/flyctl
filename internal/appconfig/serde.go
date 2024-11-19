package appconfig

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/itchyny/json2yaml"
	"github.com/pelletier/go-toml/v2"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
	"gopkg.in/yaml.v2"
)

const flyConfigHeader = `# fly.%s app configuration file generated for %s on %s
#
# See https://fly.io/docs/reference/configuration/ for information about how to use this file.
#

`

// used to detect the start of a new object or array in JSON or YAML
var startObjectOrArray = regexp.MustCompile(`^\s*"?\w+"?:( [[{])?$`)

// LoadConfig loads the app config at the given path.
func LoadConfig(path string) (cfg *Config, err error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(path, ".json") {
		cfg, err = unmarshalJSON(buf)
	} else if strings.HasSuffix(path, ".yaml") {
		cfg, err = unmarshalYAML(buf)
	} else {
		cfg, err = unmarshalTOML(buf)
	}
	if err != nil {
		return nil, err
	}

	cfg.configFilePath = path
	// cfg.WriteToFile("patched-fly.toml")
	return cfg, nil
}

func (c *Config) WriteTo(w io.Writer, format string) (int64, error) {
	var b []byte
	var err error

	if format == "json" {
		b, err = json.MarshalIndent(c, "", "  ")
	} else if format == "yaml" {
		b, err = c.MarshalAsYAML()
	} else {
		b, err = c.marshalTOML()
	}

	if err != nil {
		return 0, err
	}

	if format != "json" {
		// JSON doesn't allow comments, so we can't add a header
		_, err = fmt.Fprintf(w, flyConfigHeader, format, c.AppName, time.Now().Format(time.RFC3339))
		if err != nil {
			return 0, err
		}
	}

	if format == "toml" {
		return bytes.NewBuffer(b).WriteTo(w)
	} else {
		return prettyPrintJSONandYAML(w, b)
	}
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

	_, err = c.WriteTo(file, strings.TrimLeft(strings.ToLower(filepath.Ext(filename)), "."))
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
	if c == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(*c)
}

// MarshalAsYAML first marshals the config to JSON and then converts it to YAML
// this is done to pick up the json: struct tags; fortunately, we write
// YAML infrequently, and only on explicit user request
func (c *Config) MarshalAsYAML() ([]byte, error) {
	if c == nil {
		return json.Marshal(nil)
	}
	jsonConfig, err := json.Marshal(*c)

	if err != nil {
		return nil, err
	}

	input := bytes.NewBuffer(jsonConfig)
	var output bytes.Buffer
	err = json2yaml.Convert(&output, input)

	if err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

// MarshalAsTOML serializes the configuration to TOML format
func (c *Config) MarshalAsTOML() ([]byte, error) {
	return c.marshalTOML()
}

// marshalTOML serializes the configuration to TOML format
// NOTES:
//   - It can't be called `MarshalTOML` because toml libraries don't support marshaler interface on root values
//   - Needs to reimplements most of MarshalJSON to enforce order of fields
//   - Instead of this, you usually need one WriteTo(), WriteToFile() or WriteToDisk()
func (c *Config) marshalTOML() ([]byte, error) {
	var b bytes.Buffer
	encoder := toml.NewEncoder(&b)
	encoder.SetIndentTables(true)
	encoder.SetMarshalJsonNumbers(true)

	if c != nil {
		if err := encoder.Encode(c); err != nil {
			return nil, err
		}
	}

	return b.Bytes(), nil
}

func unmarshalTOML(buf []byte) (*Config, error) {
	cfgMap := map[string]any{}
	if err := toml.Unmarshal(buf, &cfgMap); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return nil, fmt.Errorf("row %d column %d\n%s", row, col, derr.String())
		}
		return nil, err
	}
	cfg, err := applyPatches(cfgMap)

	// In case of parsing error fallback to bare compatibility
	if err != nil {
		// Unmarshal twice due to in-place cfgMap updates performed by patches
		raw := map[string]any{}
		if err := toml.Unmarshal(buf, &raw); err != nil {
			return nil, err
		}
		cfg = &Config{v2UnmarshalError: err}
		if name, ok := (raw["app"]).(string); ok {
			cfg.AppName = name
		}
	}

	return cfg, nil
}

func unmarshalJSON(buf []byte) (*Config, error) {
	cfgMap := map[string]any{}
	if err := json.Unmarshal(buf, &cfgMap); err != nil {
		return nil, err
	}
	cfg, err := applyPatches(cfgMap)

	// In case of parsing error fallback to bare compatibility
	if err != nil {
		// Unmarshal twice due to in-place cfgMap updates performed by patches
		raw := map[string]any{}
		if err := json.Unmarshal(buf, &raw); err != nil {
			return nil, err
		}
		cfg = &Config{v2UnmarshalError: err}
		if name, ok := (raw["app"]).(string); ok {
			cfg.AppName = name
		}
	}

	return cfg, nil
}

func unmarshalYAML(buf []byte) (*Config, error) {
	cfgMap := map[string]any{}
	if err := yaml.Unmarshal(buf, &cfgMap); err != nil {
		return nil, err
	}
	stringifyYAMLMapKeys(cfgMap)
	cfg, err := applyPatches(cfgMap)

	// In case of parsing error fallback to bare compatibility
	if err != nil {
		// Unmarshal twice due to in-place cfgMap updates performed by patches
		raw := map[string]any{}
		if err := yaml.Unmarshal(buf, &raw); err != nil {
			return nil, err
		}
		cfg = &Config{v2UnmarshalError: err}
		if name, ok := (raw["app"]).(string); ok {
			cfg.AppName = name
		}
	}

	return cfg, nil
}

// stringifyYAMLMapKeys converts map keys from interface{} to string
// This is necessary because the yaml.v2 package unmarshals map keys as interface{},
// which is not compatible with TOML and JSON which unmarshal map keys as strings.
func stringifyYAMLMapKeys(obj interface{}) interface{} {
	if arrayobj, ok := obj.([]interface{}); ok {
		for i, v := range arrayobj {
			arrayobj[i] = stringifyYAMLMapKeys(v)
		}
	} else if mapobj, ok := obj.(map[string]interface{}); ok {
		for k, v := range mapobj {
			mapobj[k] = stringifyYAMLMapKeys(v)
		}
	} else if mapobj, ok := obj.(map[interface{}]interface{}); ok {
		newmap := make(map[string]interface{})
		for k, v := range mapobj {
			newmap[k.(string)] = stringifyYAMLMapKeys(v)
		}
		obj = newmap
	}

	return obj
}

// intelligently add new lines to the JSON and YAML outputs
// this is intended to roughly match the output of the toml encoder
// which is pretty-printed by default to add new lines before
// every new object or array, with the exception of the first object
func prettyPrintJSONandYAML(w io.Writer, b []byte) (int64, error) {
	// pretty print by intelligently adding newlines
	scanner := bufio.NewScanner(bytes.NewReader(b))
	section := true
	total := 0
	for scanner.Scan() {
		text := scanner.Text()

		if startObjectOrArray.MatchString(text) {
			if !section {
				count, err := w.Write([]byte("\n"))
				if err != nil {
					return 0, err
				}
				total += count
				section = true
			}
		} else {
			section = false
		}

		count, err := w.Write([]byte(text + "\n"))
		if err != nil {
			return 0, err
		}
		total += count
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return int64(total), nil
}
