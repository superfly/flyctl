package presenters

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
)

type Services struct {
	Tasks []api.Task
}

func (p *Services) FieldNames() []string {
	return []string{"Task", "Protocol", "Ports"}
}

func (p *Services) Records() []map[string]string {
	out := []map[string]string{}

	for _, task := range p.Tasks {
		for _, service := range task.Services {
			ports := []string{}
			for _, p := range service.Ports {
				ports = append(ports, fmt.Sprintf("%d => %d [%s]", p.Port, service.InternalPort, strings.Join(p.Handlers, ", ")))
			}

			out = append(out, map[string]string{
				"Task":     task.Name,
				"Protocol": service.Protocol,
				"Ports":    strings.Join(ports, "\n"),
			})
		}
	}

	return out
}
