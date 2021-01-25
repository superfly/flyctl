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
	AppName    string
	Build      *Build
	Definition map[string]interface{}
}

type Build struct {
	Builder    string
	Args       map[string]string
	Buildpacks []string
	// Or...
	Builtin  string
	Settings map[string]interface{}
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
		insection := false
		b := Build{
			Args:       map[string]string{},
			Settings:   map[string]interface{}{},
			Buildpacks: []string{},
		}
		for k, v := range buildConfig {
			switch k {
			case "builder":
				b.Builder = fmt.Sprint(v)
				insection = true
			case "buildpacks":
				if bpSlice, ok := v.([]interface{}); ok {
					for _, argV := range bpSlice {
						b.Buildpacks = append(b.Buildpacks, fmt.Sprint(argV))
					}
				}
				insection = true
			case "args":
				if argMap, ok := v.(map[string]interface{}); ok {
					for argK, argV := range argMap {
						b.Args[argK] = fmt.Sprint(argV)
					}
				}
				insection = true
			case "builtin":
				b.Builtin = fmt.Sprint(v)
				insection = true
			case "settings":
				if settingsMap, ok := v.(map[string]interface{}); ok {
					for settingK, settingV := range settingsMap {
						b.Settings[settingK] = settingV //fmt.Sprint(argV)
					}
				}
				insection = true
			case "image":
				b.Image = fmt.Sprint(v)
				insection = true
			default:
				if !insection {
					b.Args[k] = fmt.Sprint(v)
				}
			}
		}
		if b.Builder != "" || b.Builtin != "" || b.Image != "" || len(b.Args) > 0 {
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

	if ac.Build != nil {
		buildData := map[string]interface{}{}
		if ac.Build.Builder != "" {
			buildData["builder"] = ac.Build.Builder
		}
		if len(ac.Build.Buildpacks) > 0 {
			buildData["buildpacks"] = ac.Build.Buildpacks
		}
		if len(ac.Build.Args) > 0 {
			buildData["args"] = ac.Build.Args
		}
		if ac.Build.Builtin != "" {
			buildData["builtin"] = ac.Build.Builtin
			if len(ac.Build.Settings) > 0 {
				buildData["settings"] = ac.Build.Settings
			}
		}
		if ac.Build.Image != "" {
			buildData["image"] = ac.Build.Image
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

// HasServices - Does this config have a services section
func (ac *AppConfig) HasServices() bool {
	_, ok := ac.Definition["services"].([]interface{})

	return ok
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
	tmpservices, ok := ac.Definition["services"]

	if !ok {
		return -1, errors.New("could not find internal port setting")
	}

	services, ok := tmpservices.([]map[string]interface{})

	if ok {
		internalport, ok := services[0]["internal_port"].(int64)
		if ok {
			return int(internalport), nil
		}
		internalportfloat, ok := services[0]["internal_port"].(float64)
		if ok {
			return int(internalportfloat), nil
		}
	}
	return 8080, nil
}

const defaultConfigFileName = "fly.toml"

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// Is this a bare directory path? Stat the path
	pd, err := os.Stat(p)

	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return "", err
	}

	// Ok, something exists. Is it a file - yes? return the path
	if pd.IsDir() {
		return path.Join(p, defaultConfigFileName), nil
	}

	return p, nil
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
