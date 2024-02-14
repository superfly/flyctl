package appconfig

import (
	"github.com/pelletier/go-toml/v2"
	fly "github.com/superfly/fly-go"
)

func (c *Config) ToDefinition() (*fly.Definition, error) {
	var err error
	buf, err := toml.Marshal(c)
	if err != nil {
		return nil, err
	}

	definition := &fly.Definition{}
	if err := toml.Unmarshal(buf, definition); err != nil {
		return nil, err
	}
	return definition, nil
}

func FromDefinition(definition *fly.Definition) (*Config, error) {
	buf, err := toml.Marshal(*definition)
	if err != nil {
		return nil, err
	}
	return unmarshalTOML(buf)
}
