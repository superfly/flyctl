package app

import (
	"encoding/json"

	"github.com/superfly/flyctl/api"
)

func (c *Config) ToDefinition() (*api.Definition, error) {
	var err error
	buf, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	definition := &api.Definition{}
	if err := json.Unmarshal(buf, definition); err != nil {
		return nil, err
	}
	delete(*definition, "app")
	delete(*definition, "build")
	return definition, nil
}
