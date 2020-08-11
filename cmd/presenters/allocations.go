package presenters

import (
	"fmt"
	"strconv"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
)

type Allocations struct {
	Allocations   []*api.AllocationStatus
	BackupRegions []api.Region
}

func (p *Allocations) APIStruct() interface{} {
	return p.Allocations
}

func (p *Allocations) FieldNames() []string {
	return []string{"ID", "Version", "Region", "Desired", "Status", "Health Checks", "Restarts", "Created"}
}

func (p *Allocations) Records() []map[string]string {
	out := []map[string]string{}
	multipleVersions := hasMultipleVersions(p.Allocations)

	for _, alloc := range p.Allocations {
		version := strconv.Itoa(alloc.Version)
		if multipleVersions && alloc.LatestVersion {
			version = version + " " + aurora.Green("⇡").String()
		}

		region := alloc.Region
		if len(p.BackupRegions) > 0 {
			for _, r := range p.BackupRegions {
				if alloc.Region == r.Code {
					region = alloc.Region + "(B)"
					break
				}
			}
		}

		out = append(out, map[string]string{
			"ID":            alloc.IDShort,
			"Version":       version,
			"Status":        formatAllocStatus(alloc),
			"Desired":       alloc.DesiredStatus,
			"Region":        region,
			"Created":       formatRelativeTime(alloc.CreatedAt),
			"Health Checks": FormatHealthChecksSummary(alloc),
			"Restarts":      strconv.Itoa(alloc.Restarts),
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

func formatAllocStatus(alloc *api.AllocationStatus) string {
	if alloc.Transitioning {
		return aurora.Bold(alloc.Status).String()
	}
	return alloc.Status
}

func passingChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "passing" {
			n++
		}
	}
	return n
}

func warnChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "warn" {
			n++
		}
	}
	return n
}

func critChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "critical" {
			n++
		}
	}
	return n
}

func FormatDeploymentSummary(d *api.DeploymentStatus) string {
	if d.InProgress {
		return fmt.Sprintf("v%d is being deployed", d.Version)
	}
	if d.Successful {
		return fmt.Sprintf("v%d deployed successfully", d.Version)
	}

	return fmt.Sprintf("v%d %s - %s", d.Version, d.Status, d.Description)
}

func FormatDeploymentAllocSummary(d *api.DeploymentStatus) string {
	allocCounts := fmt.Sprintf("%d desired, %d placed, %d healthy, %d unhealthy", d.DesiredCount, d.PlacedCount, d.HealthyCount, d.UnhealthyCount)

	restarts := 0
	for _, alloc := range d.Allocations {
		restarts += alloc.Restarts
	}
	if restarts > 0 {
		allocCounts = fmt.Sprintf("%s [restarts: %d]", allocCounts, restarts)
	}

	checkCounts := FormatHealthChecksSummary(d.Allocations...)

	if checkCounts == "" {
		return allocCounts
	}

	return allocCounts + " [health checks: " + checkCounts + "]"
}

func FormatAllocSummary(alloc *api.AllocationStatus) string {
	msg := fmt.Sprintf("%s: %s %s", alloc.IDShort, alloc.Region, alloc.Status)

	if alloc.Status == "pending" {
		return msg
	}

	if alloc.Failed {
		msg += " failed"
	} else if alloc.Healthy {
		msg += " healthy"
	} else {
		msg += " unhealthy"
	}

	if alloc.Canary {
		msg += " [canary]"
	}

	if checkStr := FormatHealthChecksSummary(alloc); checkStr != "" {
		msg += " [health checks: " + checkStr + "]"
	}

	return msg
}

func FormatHealthChecksSummary(allocs ...*api.AllocationStatus) string {
	var total, pass, crit, warn int

	for _, alloc := range allocs {
		if n := len(alloc.Checks); n > 0 {
			total += n
			pass += passingChecks(alloc.Checks)
			crit += critChecks(alloc.Checks)
			warn += warnChecks(alloc.Checks)
		}
	}

	if total == 0 {
		return ""
	}

	checkStr := fmt.Sprintf("%d total", total)

	if pass > 0 {
		checkStr += ", " + fmt.Sprintf("%d passing", pass)
	}
	if warn > 0 {
		checkStr += ", " + fmt.Sprintf("%d warning", warn)
	}
	if crit > 0 {
		checkStr += ", " + fmt.Sprintf("%d critical", crit)
	}

	return checkStr
}
