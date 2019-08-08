package presenters

import "github.com/superfly/flyctl/api"

type TaskSummary struct {
	Tasks []api.Task
}

func (p *TaskSummary) FieldNames() []string {
	return []string{"Name", "Status", "Services"}
}

func (p *TaskSummary) Records() []map[string]string {
	out := []map[string]string{}

	for _, service := range p.Tasks {
		out = append(out, map[string]string{
			"Name":     service.Name,
			"Status":   service.Status,
			"Services": service.ServicesSummary,
		})
	}

	return out
}
