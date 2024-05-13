package appconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
	"gopkg.in/yaml.v2"
)

const flytomlHeader = `# fly.toml app configuration file generated for %s on %s
#
# See https://fly.io/docs/reference/configuration/ for information about how to use this file.
#

`

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

func (c *Config) WriteTo(w io.Writer) (int64, error) {
	b, err := c.marshalTOML()
	if err != nil {
		return 0, err
	}
	_, err = fmt.Fprintf(w, flytomlHeader, c.AppName, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return bytes.NewBuffer(b).WriteTo(w)
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

	_, err = c.WriteTo(file)
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
