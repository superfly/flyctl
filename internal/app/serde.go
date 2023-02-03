package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
)

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

	cfg, err = unmarshalTOML(file)
	cfg.FlyTomlPath = path
	return
}

func (c *Config) EncodeTo(w io.Writer) error {
	return c.marshalTOML(w)
}

func (c *Config) unmarshalTOML(r io.ReadSeeker) error {
	var definition map[string]any
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
		buildData := make(map[string]any)
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
func normalizeDefinition(src map[string]any) (map[string]any, error) {
	if len(src) == 0 {
		return src, nil
	}

	dst := make(map[string]any)

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
