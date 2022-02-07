package presenters

import (
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
	return []string{"Timestamp", "Kind"}
}

// Records - formats allocation events into a map
func (p *MachineEvents) Records() []map[string]string {
	out := []map[string]string{}

	for _, event := range p.Events {
		out = append(out, map[string]string{
			"Kind":      event.Kind,
			"Timestamp": event.Timestamp.Format(time.RFC3339),
			// "Message":   event.Message,
		})
	}

	return out
}
