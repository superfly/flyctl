package flyctl

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/terminal"
)

type Manifest struct {
	AppName string `toml:"app"`
}

func DefaultManifestPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		terminal.Error(err)
		return ""
	}

	return path.Join(cwd, "fly.toml")
}

func LoadManifest(path string) (Manifest, error) {
	var out Manifest

	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}

	if _, err := toml.Decode(string(data), &out); err != nil {
		log.Fatalln(err)
		return out, err
	}

	return out, nil
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
