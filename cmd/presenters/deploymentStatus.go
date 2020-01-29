package presenters

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

type DeploymentStatus struct {
	Status *api.DeploymentStatus
}

func (p *DeploymentStatus) FieldNames() []string {
	return []string{"ID", "Version", "Status", "Description", "Allocations"}
}

func (p *DeploymentStatus) Records() []map[string]string {
	out := []map[string]string{}

	out = append(out, map[string]string{
		"ID":          p.Status.ID,
		"Version":     fmt.Sprintf("v%d", p.Status.Version),
		"Status":      p.Status.Status,
		"Description": p.Status.Description,
		"Allocations": formatDeploymentAllocations(p.Status),
	})

	return out
}

func formatDeploymentAllocations(d *api.DeploymentStatus) string {
	return fmt.Sprintf("%d desired, %d placed, %d healthy, %d unhealthy",
		d.DesiredCount, d.PlacedCount, d.HealthyCount, d.UnhealthyCount)
}
