package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type Allocations struct {
	Tasks []api.Task
}

func (p *Allocations) FieldNames() []string {
	return []string{"ID", "Version", "Task", "Region", "Desired", "Status", "Created"}
}

func (p *Allocations) Records() []map[string]string {
	out := []map[string]string{}

	for _, task := range p.Tasks {
		for _, alloc := range task.Allocations {
			out = append(out, map[string]string{
				"ID":      alloc.ID,
				"Version": strconv.Itoa(alloc.Version),
				"Task":    task.Name,
				"Status":  alloc.Status,
				"Desired": alloc.DesiredStatus,
				"Region":  alloc.Region,
				"Created": formatRelativeTime(alloc.CreatedAt),
			})
		}
	}

	return out
}
