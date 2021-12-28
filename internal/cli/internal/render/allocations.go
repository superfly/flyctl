package render

import (
	"io"
	"strconv"
	"time"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/format"
)

func AllocationStatuses(w io.Writer, title string, backupRegions []api.Region, statuses ...*api.AllocationStatus) error {
	multipleVersions := hasMultipleVersions(statuses)

	var rows [][]string
	for _, alloc := range statuses {
		version := strconv.Itoa(alloc.Version)
		if multipleVersions && alloc.LatestVersion {
			version = version + " " + aurora.Green("⇡").String()
		}

		region := alloc.Region
		if len(backupRegions) > 0 {
			for _, r := range backupRegions {
				if alloc.Region == r.Code {
					region = alloc.Region + "(B)"
					break
				}
			}
		}

		rows = append(rows, []string{
			alloc.IDShort,                        // ID,
			alloc.TaskName,                       // Process
			version,                              // Version
			region,                               // Region
			alloc.DesiredStatus,                  // Desired
			format.AllocStatus(alloc),            // Status
			format.HealthChecksSummary(alloc),    // Health Checks
			strconv.Itoa(alloc.Restarts),         // Restarts
			format.RelativeTime(alloc.CreatedAt), // Created
		})
	}

	return Table(w, title, rows,
		"ID",
		"Process",
		"Version",
		"Region",
		"Desired",
		"Status",
		"Health Checks",
		"Restarts",
		"Created",
	)
}

func hasMultipleVersions(allocations []*api.AllocationStatus) bool {
	if len(allocations) == 0 {
		return false
	}

	v := allocations[0].Version

	for _, a := range allocations[1:] {
		if v != a.Version {
			return true
		}
	}

	return false
}

func AllocationEvents(w io.Writer, title string, events ...api.AllocationEvent) error {
	var rows [][]string

	for _, evt := range events {
		rows = append(rows, []string{
			evt.Timestamp.Format(time.RFC3339),
			evt.Type,
			evt.Message,
		})
	}

	return Table(w, title, rows, "Timestamp", "Type", "Message")
}
