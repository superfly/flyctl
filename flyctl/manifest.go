package flyctl

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/terminal"
)

type Manifest struct {
	AppName string            `toml:"app"`
	Build   map[string]string `toml:"build"`
}

func (m *Manifest) Builder() string {
	return m.Build["builder"]
}

func DefaultManifestPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		terminal.Error(err)
		return ""
	}

	return path.Join(cwd, "fly.toml")
}

func LoadManifestFromReader(reader io.Reader) (*Manifest, error) {
	var out Manifest

	if _, err := toml.DecodeReader(reader, &out); err != nil {
		log.Fatalln(err)
		return nil, err
	}

	return &out, nil
}

func LoadManifest(path string) (*Manifest, error) {
	file, err := os.Open(path)
	switch {
	case os.IsNotExist(err):
		return nil, nil
	case err != nil:
		return nil, err
	}

	return LoadManifestFromReader(file)
}

func (manifest *Manifest) RenderToString() string {
	var buf bytes.Buffer
	toml.NewEncoder(bufio.NewWriter(&buf)).Encode(&manifest)
	return buf.String()
}

func (manifest *Manifest) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(&manifest)
}
