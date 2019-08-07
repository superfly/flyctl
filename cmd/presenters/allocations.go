package presenters

import "github.com/superfly/flyctl/api"

type Allocations struct {
	Services []api.Service
}

func (p *Allocations) FieldNames() []string {
	return []string{"ID", "Service", "Region", "Desired", "Status", "Created", "Modified"}
}

func (p *Allocations) FieldMap() map[string]string {
	return map[string]string{
		"ID":       "ID",
		"Service":  "Service",
		"Status":   "Status",
		"Desired":  "Desired",
		"Region":   "Region",
		"Created":  "Created",
		"Modified": "Modified",
	}
}

func (p *Allocations) Records() []map[string]string {
	out := []map[string]string{}

	for _, service := range p.Services {
		for _, alloc := range service.Allocations {
			out = append(out, map[string]string{
				"ID":       alloc.ID,
				"Service":  service.Name,
				"Status":   alloc.Status,
				"Desired":  alloc.DesiredStatus,
				"Region":   alloc.Region,
				"Created":  formatRelativeTime(alloc.CreatedAt),
				"Modified": formatRelativeTime(alloc.UpdatedAt),
			})
		}
	}

	return out
}
