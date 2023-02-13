package presenters

import (
	"fmt"
)

type Environment struct {
	Envs map[string]interface{}
}

func (p *Environment) APIStruct() interface{} {
	return nil
}

func (p *Environment) FieldNames() []string {
	return []string{"Name", "Value"}
}

func (p *Environment) Records() []map[string]string {
	out := []map[string]string{}

	for key, value := range p.Envs {
		out = append(out, map[string]string{
			"Name":  key,
			"Value": fmt.Sprintf("%v", value),
		})
	}
	return out
}
