package presenters

import (
	"strconv"

	"github.com/logrusorgru/aurora"
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

	multipleVersions := hasMultipleVersions(p.Tasks)

	for _, task := range p.Tasks {
		for _, alloc := range task.Allocations {
			version := strconv.Itoa(alloc.Version)
			if multipleVersions && alloc.LatestVersion {
				version = version + " " + aurora.Green("⇡").String()
			}

			out = append(out, map[string]string{
				"ID":      alloc.ID,
				"Version": version,
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

func hasMultipleVersions(tasks []api.Task) bool {
	var v int
	for _, task := range tasks {
		for _, alloc := range task.Allocations {
			if v != 0 && v != alloc.Version {
				return true
			}
			v = alloc.Version
		}
	}

	return false
}
