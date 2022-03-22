package render

import (
	"io"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/superfly/flyctl/api"
)

func MachineStatus(w io.Writer, machine *api.Machine) error {
	var rows [][]string

	rows = append(rows, []string{
		machine.ID,
		machine.Name,
		machine.State,
		machine.Region,
		humanize.Time(machine.CreatedAt),
		machine.App.Hostname,
	})

	return VerticalTable(w, "Machine", rows,
		"ID",
		"Name",
		"State",
		"Region",
		"Created",
		"Hostname",
	)

}

func MachineStatuses(w io.Writer, title string, machines ...*api.Machine) error {
	var rows [][]string

	for _, machine := range machines {
		rows = append(rows, []string{
			machine.ID,
			machine.Name,
			machine.Region,
			machine.State,
			machine.CreatedAt.Format(time.RFC3339),
		})
	}

	return Table(w, title, rows,
		"ID",
		"Name",
		"Region",
		"State",
		"Created",
	)
}

func MachineIPs(w io.Writer, ips ...*api.MachineIP) error {
	var rows [][]string

	for _, ip := range ips {
		if ip.Kind != "privatenet" {
			continue
		}

		rows = append(rows, []string{
			ip.Family,
			ip.IP,
			ip.Kind,
		})
	}

	return Table(w, "Machine IPs", rows,
		"Family",
		"Address",
		"Kind",
	)
}

func MachineEvents(w io.Writer, events ...*api.MachineEvent) error {
	var rows [][]string

	for _, evt := range events {
		// var exitMeta string

		// if evt.Metadata != nil {
		// 	for k, v := range evt.Metadata {
		// 		exitMeta += fmt.Sprintf("%s: %v, ", k, v)
		// 	}
		// }
		// if exitMeta == "" {
		// 	exitMeta = "N/A"
		// }
		rows = append(rows, []string{
			evt.ID,
			evt.Kind,
			evt.Timestamp.Format(time.RFC3339),
			// exitMeta,
		})
	}

	return Table(w, "Recent Events", rows, "ID", "Kind", "Timestamp")
}
