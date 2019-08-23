package presenters

import (
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
)

type Services struct {
	Tasks []api.Task
}

func (p *Services) FieldNames() []string {
	return []string{"Task", "Protocol", "Port", "Internal Port", "Handlers"}
}

func (p *Services) Records() []map[string]string {
	out := []map[string]string{}

	for _, task := range p.Tasks {
		for _, service := range task.Services {
			out = append(out, map[string]string{
				"Task":          task.Name,
				"Protocol":      service.Protocol,
				"Port":          strconv.Itoa(service.Port),
				"Internal Port": strconv.Itoa(service.InternalPort),
				"Handlers":      strings.Join(service.Filters, " "),
			})
		}
	}

	return out
}
