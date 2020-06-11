package presenters

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
)

type Services struct {
	Services []api.Service
}

func (p *Services) APIStruct() interface{} {
	return p.Services
}

func (p *Services) FieldNames() []string {
	return []string{"Protocol", "Ports"}
}

func (p *Services) Records() []map[string]string {
	out := []map[string]string{}

	for _, service := range p.Services {
		ports := []string{}
		for _, p := range service.Ports {
			ports = append(ports, fmt.Sprintf("%d => %d [%s]", p.Port, service.InternalPort, strings.Join(p.Handlers, ", ")))
		}

		out = append(out, map[string]string{
			"Protocol": service.Protocol,
			"Ports":    strings.Join(ports, "\n"),
		})
	}

	return out
}
