package render

import (
	"io"
	"time"

	"github.com/superfly/flyctl/api"
)

func MachineStatuses(w io.Writer, title string, machines ...*api.Machine) error {
	var rows [][]string

	for _, machine := range machines {
		rows = append(rows, []string{
			machine.ID,
			machine.Name,
			machine.State,
			machine.Region,
			machine.CreatedAt.Format(time.RFC3339),
		})
	}

	return Table(w, title, rows,
		"ID",
		"Name",
		"Status",
		"Region",
		"Created",
	)
}
