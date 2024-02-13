package appconfig

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/superfly/fly-go/api"
)

func (c *Config) ToDefinition() (*api.Definition, error) {
	var err error
	buf, err := toml.Marshal(c)
	if err != nil {
		return nil, err
	}

	definition := &api.Definition{}
	if err := toml.Unmarshal(buf, definition); err != nil {
		return nil, err
	}
	return definition, nil
}

func FromDefinition(definition *api.Definition) (*Config, error) {
	buf, err := toml.Marshal(*definition)
	if err != nil {
		return nil, err
	}
	return unmarshalTOML(buf)
}
