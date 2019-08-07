package presenters

import "github.com/superfly/flyctl/api"

type Services struct {
	Services []api.Service
}

func (p *Services) FieldNames() []string {
	return []string{"Name", "Status"}
}

func (p *Services) FieldMap() map[string]string {
	return map[string]string{
		"Name":   "Name",
		"Status": "Status",
	}
}

func (p *Services) Records() []map[string]string {
	out := []map[string]string{}

	for _, service := range p.Services {
		out = append(out, map[string]string{
			"Name":   service.Name,
			"Status": service.Status,
		})
	}

	return out
}
