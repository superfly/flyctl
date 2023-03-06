package appv2

import (
	"github.com/pelletier/go-toml"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
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
	delete(*definition, "app")
	delete(*definition, "build")
	delete(*definition, "primary_region")
	delete(*definition, "http_service")
	return definition, nil
}

func FromDefinition(definition *api.Definition) (*Config, error) {
	hash := map[string]any{}
	for k, v := range *definition {
		hash[k] = v
	}
	return applyPatches(hash)
}

// SanitizedDefinition returns a definition cleaned from any extra fields
// not valid for Web API GQL endpoints.
func (c *Config) SanitizedDefinition() map[string]any {
	// Beware this is a shallow Copy
	definition := lo.Assign(c.RawDefinition)
	delete(definition, "app")
	delete(definition, "build")
	delete(definition, "primary_region")
	delete(definition, "http_service")
	return definition
}
