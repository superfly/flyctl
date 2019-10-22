package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type DeploymentTaskStatus struct {
	Release api.Release
}

func (p *DeploymentTaskStatus) FieldNames() []string {
	if p.Release.DeploymentStrategy == "canary" {
		return []string{"Name", "Promoted", "Desired", "Canaries", "Placed", "Healthy", "Unhealthy", "Progress Deadline"}
	}
	return []string{"Name", "Desired", "Placed", "Healthy", "Unhealthy", "Progress Deadline"}
}

func (p *DeploymentTaskStatus) Records() []map[string]string {
	out := []map[string]string{}

	for _, task := range p.Release.Deployment.Tasks {
		out = append(out, map[string]string{
			"Name":              task.Name,
			"Promoted":          strconv.FormatBool(task.Promoted),
			"Progress Deadline": formatRelativeTime(task.ProgressDeadline),
			"Desired":           strconv.Itoa(task.Desired),
			"Canaries":          strconv.Itoa(task.Canaries),
			"Placed":            strconv.Itoa(task.Placed),
			"Healthy":           strconv.Itoa(task.Healthy),
			"Unhealthy":         strconv.Itoa(task.Unhealthy),
		})
	}

	return out
}
