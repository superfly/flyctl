package app

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

	cfg.FlyTomlPath = path
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
	cfgMap := map[string]any{}
	if err := toml.Unmarshal(buf, &cfgMap); err != nil {
		return nil, err
	}

	return applyPatches(cfgMap)
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
