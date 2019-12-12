package flyctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/helpers"
)

type ConfigFormat string

const (
	TOMLFormat        ConfigFormat = ".toml"
	JSONFormat                     = ".json"
	UnsupportedFormat              = ""
)

type AppConfig struct {
	AppName string
	Build   *Build

	Definition map[string]interface{}
}

type Build struct {
	Builder string
	Args    map[string]string
}

func NewAppConfig() *AppConfig {
	return &AppConfig{
		Definition: map[string]interface{}{},
	}
}

func LoadAppConfig(configFile string) (*AppConfig, error) {
	fullConfigFilePath, err := filepath.Abs(configFile)
	if err != nil {
		return nil, err
	}

	appConfig := AppConfig{
		Definition: map[string]interface{}{},
	}

	file, err := os.Open(fullConfigFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	switch ConfigFormatFromPath(fullConfigFilePath) {
	case TOMLFormat:
		err = appConfig.unmarshalTOML(file)
	default:
		return nil, errors.New("Unsupported config file format")
	}

	return &appConfig, err
}

func (ac *AppConfig) HasDefinition() bool {
	return len(ac.Definition) > 0
}

func (ac *AppConfig) WriteTo(w io.Writer, format ConfigFormat) error {
	switch format {
	case TOMLFormat:
		return ac.marshalTOML(w)
	case JSONFormat:
		return ac.marshalJSON(w)
	}

	return nil
}

func (ac *AppConfig) unmarshalTOML(r io.Reader) error {
	var data map[string]interface{}

	if _, err := toml.DecodeReader(r, &data); err != nil {
		return err
	}

	if appName, ok := (data["app"]).(string); ok {
		ac.AppName = appName
	}
	delete(data, "app")

	if buildConfig, ok := (data["build"]).(map[string]interface{}); ok {
		b := Build{
			Args: map[string]string{},
		}
		for k, v := range buildConfig {
			if k == "builder" {
				b.Builder = fmt.Sprint(v)
			} else if k == "args" {
				argsBlock, ok := v.(map[string]string)
				if ok {
					for ak, av := range argsBlock {
						b.Args[ak] = fmt.Sprint(av)
					}
				}
			} else {
				b.Args[k] = fmt.Sprint(v)
			}
		}
		if b.Builder != "" {
			ac.Build = &b
		}
	}
	delete(data, "build")

	ac.Definition = data

	return nil
}

func (ac AppConfig) marshalTOML(w io.Writer) error {
	encoder := toml.NewEncoder(w)

	rawData := map[string]interface{}{
		"app": ac.AppName,
	}

	if ac.Build != nil && ac.Build.Builder != "" {
		buildData := map[string]interface{}{
			"builder": ac.Build.Builder,
		}
		if len(ac.Build.Args) > 0 {
			buildData["args"] = ac.Build.Args
		}
		rawData["build"] = buildData
	}

	if err := encoder.Encode(rawData); err != nil {
		return err
	}

	if len(ac.Definition) > 0 {
		if err := encoder.Encode(ac.Definition); err != nil {
			return err
		}
	}

	return nil
}

func (ac *AppConfig) marshalJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(ac)
}

func (ac *AppConfig) WriteToFile(filename string) error {
	if err := helpers.MkdirAll(filename); err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return ac.WriteTo(file, ConfigFormatFromPath(filename))
}

const defaultConfigFileName = "fly.toml"

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// path is a file, return
	if filepath.Ext(p) != "" {
		return p, nil
	}

	return path.Join(p, defaultConfigFileName), nil

	// if helpers.FileExists(path.Join(p, defaultConfigFileName)) {
	// 	return path.Join(p, defaultConfigFileName), nil
	// }

	// if helpers.FileExists(path.Join(p, deprecatedConfigFileName)) {
	// 	return path.Join(p, deprecatedConfigFileName), nil
	// }

	// return "", nil
}

func ConfigFormatFromPath(p string) ConfigFormat {
	switch path.Ext(p) {
	case ".toml":
		return TOMLFormat
	case ".json":
		return JSONFormat
	}
	return UnsupportedFormat
}

func ConfigFileExistsAtPath(p string) (bool, error) {
	p, err := ResolveConfigFileFromPath(p)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	return !os.IsNotExist(err), nil
}
