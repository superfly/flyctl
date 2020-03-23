package presenters

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
)

type Allocations struct {
	Allocations []*api.AllocationStatus
}

func (p *Allocations) FieldNames() []string {
	return []string{"ID", "Version", "Region", "Desired", "Status", "Health Checks", "Created"}
}

func (p *Allocations) Records() []map[string]string {
	out := []map[string]string{}

	multipleVersions := hasMultipleVersions(p.Allocations)

	for _, alloc := range p.Allocations {
		version := strconv.Itoa(alloc.Version)
		if multipleVersions && alloc.LatestVersion {
			version = version + " " + aurora.Green("⇡").String()
		}

		out = append(out, map[string]string{
			"ID":            alloc.IDShort,
			"Version":       version,
			"Status":        formatStatus(alloc),
			"Desired":       alloc.DesiredStatus,
			"Region":        alloc.Region,
			"Created":       formatRelativeTime(alloc.CreatedAt),
			"Health Checks": formatHealthChecksSummary(alloc),
		})
	}

	return out
}

func hasMultipleVersions(allocations []*api.AllocationStatus) bool {
	var v int
	for _, alloc := range allocations {
		if v != 0 && v != alloc.Version {
			return true
		}
		v = alloc.Version
	}

	return false
}

func formatHealthChecksSummary(alloc *api.AllocationStatus) string {
	if alloc.PassingCheckCount+alloc.WarningCheckCount+alloc.CriticalCheckCount == 0 {
		return ""
	}

	var b strings.Builder

	if alloc.PassingCheckCount > 0 {
		fmt.Fprintf(&b, "%d passing", alloc.PassingCheckCount)
	}

	if alloc.WarningCheckCount > 0 {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d warning", alloc.WarningCheckCount)
	}

	if alloc.CriticalCheckCount > 0 {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d critical", alloc.CriticalCheckCount)
	}

	return b.String()
}

func formatStatus(alloc *api.AllocationStatus) string {
	if alloc.Transitioning {
		return aurora.Bold(alloc.Status).String()
	}
	return alloc.Status
}
