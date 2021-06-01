package presenters

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

type Environment struct {
	Secrets []api.Secret
	Envs    map[string]interface{}
}

func (p *Environment) APIStruct() interface{} {
	return nil
}
func (p *Environment) FieldNames() []string {
	return []string{"Name", "Type", "Value"}
}

func (p *Environment) Records() []map[string]string {
	out := []map[string]string{}

	for _, secret := range p.Secrets {
		out = append(out, map[string]string{
			"Name":  secret.Name,
			"Type":  "Secret",
			"Value": "REDACTED",
		})
	}

	for key, value := range p.Envs {
		out = append(out, map[string]string{
			"Name":  key,
			"Type":  "Variable",
			"Value": fmt.Sprintf("%v", value),
		})
	}
	return out
}
