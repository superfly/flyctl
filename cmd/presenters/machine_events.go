package presenters

import (
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
)

// MachineEvents - Holds events for an allocation
type MachineEvents struct {
	Events []*api.MachineEvent
}

// APIStruct - returns an interface to allocation events
func (p *MachineEvents) APIStruct() interface{} {
	return p.Events
}

// FieldNames - returns the field names for an allocation event
func (p *MachineEvents) FieldNames() []string {
	return []string{"Timestamp", "Id", "Kind", `Exit Code`}
}

// Records - formats allocation events into a map
func (p *MachineEvents) Records() []map[string]string {
	out := []map[string]string{}

	for _, event := range p.Events {

		var exitCode string

		if event.ExitCode != nil {
			exitCode = fmt.Sprintf("%d", *event.ExitCode)
		}

		out = append(out, map[string]string{
			"Id":        event.ID,
			"Kind":      event.Kind,
			"Timestamp": event.Timestamp.Format(time.RFC3339),
			`Exit Code`: exitCode,
		})
	}

	return out
}
