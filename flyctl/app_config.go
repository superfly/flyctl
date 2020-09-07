package flyctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/superfly/flyctl/helpers"
)

type ConfigFormat string

const (
	TOMLFormat        ConfigFormat = ".toml"
	UnsupportedFormat              = ""
)

type AppConfig struct {
	AppName string
	Build   *Build

	Definition map[string]interface{}
}

type Build struct {
	Builder    string
	Args       map[string]string
	Buildpacks []string
	// Or...
	Builtin string
	// Or...
	Image string
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

func (ac *AppConfig) HasBuilder() bool {
	return ac.Build != nil && ac.Build.Builder != ""
}

func (ac *AppConfig) HasBuiltin() bool {
	return ac.Build != nil && ac.Build.Builtin != ""
}

func (ac *AppConfig) WriteTo(w io.Writer, format ConfigFormat) error {
	switch format {
	case TOMLFormat:
		return ac.marshalTOML(w)
	}

	return fmt.Errorf("Unsupported format: %s", format)
}

func (ac *AppConfig) unmarshalTOML(r io.Reader) error {
	var data map[string]interface{}

	if _, err := toml.DecodeReader(r, &data); err != nil {
		return err
	}

	return ac.unmarshalNativeMap(data)
}

func (ac *AppConfig) unmarshalNativeMap(data map[string]interface{}) error {
	if appName, ok := (data["app"]).(string); ok {
		ac.AppName = appName
	}
	delete(data, "app")

	if buildConfig, ok := (data["build"]).(map[string]interface{}); ok {
		b := Build{
			Args:       map[string]string{},
			Buildpacks: []string{},
		}
		for k, v := range buildConfig {
			switch k {
			case "builder":
				b.Builder = fmt.Sprint(v)
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
			case "image":
				b.Image = fmt.Sprint(v)
			default:
				b.Args[k] = fmt.Sprint(v)
			}
		}
		if b.Builder != "" || b.Builtin != "" || b.Image != "" {
			ac.Build = &b
		}
	}
	delete(data, "build")

	ac.Definition = data

	return nil
}

func (ac AppConfig) marshalTOML(w io.Writer) error {
	encoder := toml.NewEncoder(w)

	fmt.Fprintf(w, "# fly.toml file generated for %s on %s\n\n", ac.AppName, time.Now().Format(time.RFC3339))

	rawData := map[string]interface{}{
		"app": ac.AppName,
	}

	if ac.Build != nil && ac.Build.Builder != "" {
		buildData := map[string]interface{}{
			"builder": ac.Build.Builder,
		}
		if len(ac.Build.Buildpacks) > 0 {
			buildData["buildpacks"] = ac.Build.Buildpacks
		}
		if len(ac.Build.Args) > 0 {
			buildData["args"] = ac.Build.Args
		}
		rawData["build"] = buildData
	} else if ac.Build != nil && ac.Build.Builtin != "" {
		buildData := map[string]interface{}{
			"builtin": ac.Build.Builtin,
		}
		rawData["build"] = buildData
	} else if ac.Build != nil && ac.Build.Image != "" {
		buildData := map[string]interface{}{
			"image": ac.Build.Image,
		}
		rawData["build"] = buildData
	}

	if err := encoder.Encode(rawData); err != nil {
		return err
	}

	if len(ac.Definition) > 0 {
		// roundtrip through json encoder to convert float64 numbers to json.Number, otherwise numbers are floats in toml
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(ac.Definition)
		d := json.NewDecoder(&buf)
		d.UseNumber()
		if err := d.Decode(&ac.Definition); err != nil {
			return err
		}

		if err := encoder.Encode(ac.Definition); err != nil {
			return err
		}
	}

	return nil
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

func (ac *AppConfig) SetInternalPort(port int) bool {
	if services, ok := ac.Definition["services"].([]interface{}); ok {
		if len(services) == 0 {
			return false
		}

		if service, ok := services[0].(map[string]interface{}); ok {
			service["internal_port"] = port
			return true
		}
	}

	return false
}

func (ac *AppConfig) GetInternalPort() (int, error) {
	services, ok := ac.Definition["services"].([]map[string]interface{})
	if ok {
		service0 := services[0] //.(map[string]interface{})
		internalport, ok := service0["internal_port"].(int64)
		if ok {
			return int(internalport), nil
		}
	}

	return -1, errors.New("could not find internal port setting")
}

const defaultConfigFileName = "fly.toml"

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// Is this a bare directory path? Stat the path
	pd, err := os.Stat(p)

	if err == os.ErrNotExist {
		return path.Join(p, defaultConfigFileName), nil
	} else if err != nil {
		return "", err
	}

	// Ok, something exists. Is it a file - yes? return the path
	if !pd.IsDir() {
		return p, nil
	}

	return path.Join(p, defaultConfigFileName), nil
}

func ConfigFormatFromPath(p string) ConfigFormat {
	switch path.Ext(p) {
	case ".toml":
		return TOMLFormat
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
