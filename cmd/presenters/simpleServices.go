package presenters

import (
	"github.com/superfly/flyctl/api"
)

type SimpleServices struct {
	Services []api.Service
}

func (p *SimpleServices) APIStruct() interface{} {
	return p.Services
}

func (p *SimpleServices) FieldNames() []string {
	return []string{"Description"}
}

func (p *SimpleServices) Records() []map[string]string {
	out := []map[string]string{}

	for _, service := range p.Services {
		out = append(out, map[string]string{
			"Description": service.Description,
		})
	}

	return out
}
