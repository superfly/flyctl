package render

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/logs"
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

func AllocationStatus(w io.Writer, title string, status *api.AllocationStatus) error {
	var rows [][]string

	rows = append(rows, []string{
		status.IDShort,
		status.TaskName,
		strconv.Itoa(status.Version),
		status.Region,
		status.DesiredStatus,
		format.AllocStatus(status),
		format.HealthChecksSummary(status),
		strconv.Itoa(status.Restarts),
		format.RelativeTime(status.CreatedAt),
	})
	return VerticalTable(w, title, rows, "ID", "Process", "Version", "Region", "Desired", "Status", "Health Checks", "Restarts", "Created")
}

func AllocationChecks(w io.Writer, title string, checks ...api.CheckState) error {
	var rows [][]string

	for _, check := range checks {
		rows = append(rows, []string{
			check.Name,
			check.ServiceName,
			check.Status,
			check.Output,
		})
	}

	return Table(w, title, rows, "ID", "Service", "State", "Output")
}

func AllocationLogs(w io.Writer, title string, entries []logs.LogEntry) error {
	fmt.Fprintln(w, aurora.Bold(title))

	for _, e := range entries {
		if err := LogEntry(w, e); err != nil {
			return err
		}
	}
	return nil
}
