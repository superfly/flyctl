package manifest

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"
)

type Manifest struct {
	AppID string `toml:"app_id"`
}

func LoadManifest(path string) (Manifest, error) {
	var out Manifest

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return out, err
	}

	if _, err := toml.Decode(string(data), &out); err != nil {
		log.Fatalln(err)
		return out, err
	}

	return out, nil
}

func SaveManifest(path string, manifest Manifest) error {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(&manifest)
}
