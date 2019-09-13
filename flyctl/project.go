package flyctl

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/viper"
)

type Project struct {
	ProjectDir string
	cfg        *viper.Viper
}

func NewProject(configFile string) *Project {
	v := viper.New()
	v.SetConfigFile(configFile)

	return &Project{
		cfg:        v,
		ProjectDir: path.Dir(configFile),
	}
}

func LoadProject(configFile string) (*Project, error) {
	configFile, err := ResolveConfigFileFromPath(configFile)
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetConfigType("toml")

	p := &Project{
		cfg:        v,
		ProjectDir: path.Dir(configFile),
	}

	if err := v.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return p, nil
}

func (p *Project) WriteConfig() error {
	if err := os.MkdirAll(path.Dir(p.cfg.ConfigFileUsed()), 0777); err != nil {
		return err
	}

	return p.cfg.WriteConfig()
}

func (p *Project) SafeWriteConfig() error {
	err := p.cfg.SafeWriteConfig()
	if os.IsNotExist(err) {
		return p.WriteConfig()
	}
	return err
}

func (p *Project) WriteConfigToPath(filename string) error {
	p.cfg.SetConfigFile(filename)
	return p.cfg.WriteConfigAs(filename)
}

func (p *Project) WriteConfigAsString() string {
	var buf bytes.Buffer
	toml.NewEncoder(bufio.NewWriter(&buf)).Encode(p.cfg.AllSettings())
	return buf.String()
}

func (p *Project) ConfigFilePath() string {
	return p.cfg.ConfigFileUsed()
}

func (p *Project) AppName() string {
	return p.cfg.GetString("app")
}

func (p *Project) SetAppName(name string) {
	p.cfg.Set("app", name)
}

func (p *Project) HasBuildConfig() bool {
	return p.cfg.IsSet("build")
}

func (p *Project) Builder() string {
	return p.cfg.GetString("build.builder")
}

func (p *Project) SetBuilder(name string) {
	p.cfg.Set("build.builder", name)
}

func (p *Project) BuildArgs() map[string]string {
	args := p.cfg.GetStringMapString("build")
	delete(args, "builder")
	return args
}

func (p *Project) Services() []Service {
	services := []Service{}
	p.cfg.UnmarshalKey("services", &services)
	return services
}

func (p *Project) SetServices(services []Service) {
	s := []map[string]interface{}{}

	for _, x := range services {
		s = append(s, map[string]interface{}{
			"protocol":      x.Protocol,
			"port":          x.Port,
			"internal_port": x.InternalPort,
			"handlers":      x.Handlers,
		})
	}

	p.cfg.Set("services", s)
}

type Service struct {
	Protocol     string   `mapstructure:"protocol"`
	Port         int      `mapstructure:"port"`
	InternalPort int      `mapstructure:"internal_port"`
	Handlers     []string `mapstructure:"handlers"`
}

const defaultConfigFileName = "fly.toml"

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// path is a directory, append default config file name
	if filepath.Ext(p) == "" {
		p = path.Join(p, defaultConfigFileName)
	}

	return p, nil
}

func ConfigFileExistsAtPath(p string) (bool, error) {
	p, err := ResolveConfigFileFromPath(p)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	return !os.IsNotExist(err), nil
}
