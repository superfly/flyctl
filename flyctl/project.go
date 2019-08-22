package flyctl

import (
	"bufio"
	"bytes"
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/spf13/viper"
)

type Project struct {
	ProjectDir string
	cfg        *viper.Viper
}

func NewProject(projectDir string) *Project {
	v := viper.New()
	file := path.Join(projectDir, "fly.toml")
	v.SetConfigFile(file)

	return &Project{
		cfg:        v,
		ProjectDir: projectDir,
	}
}

func LoadProject(projectDir string) (*Project, error) {
	v := viper.New()
	file := path.Join(projectDir, "fly.toml")
	v.SetConfigFile(file)

	if err := v.ReadInConfig(); err != nil && os.IsNotExist(err) {
		return nil, err
	}

	p := &Project{
		cfg:        v,
		ProjectDir: projectDir,
	}

	return p, nil
}

func (p *Project) WriteConfig() error {
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

func (c *Project) AppName() string {
	return c.cfg.GetString("app")
}

func (c *Project) SetAppName(name string) {
	c.cfg.Set("app", name)
}

func (c *Project) HasBuildConfig() bool {
	return c.cfg.IsSet("build")
}

func (c *Project) Builder() string {
	return c.cfg.GetString("build.builder")
}

func (c *Project) SetBuilder(name string) {
	c.cfg.Set("build.builder", name)
}

func (c *Project) BuildArgs() map[string]string {
	args := c.cfg.GetStringMapString("build")
	delete(args, "builder")
	return args
}
