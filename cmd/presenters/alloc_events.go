package presenters

import (
	"time"

	"github.com/superfly/flyctl/api"
)

// AllocationEvents - Holds events for an allocation
type AllocationEvents struct {
	Events []api.AllocationEvent
}

// APIStruct - returns an interface to allocation events
func (p *AllocationEvents) APIStruct() interface{} {
	return p.Events
}

// FieldNames - returns the field names for an allocation event
func (p *AllocationEvents) FieldNames() []string {
	return []string{"Timestamp", "Type", "Message"}
}

// Records - formats allocation events into a map
func (p *AllocationEvents) Records() []map[string]string {
	out := []map[string]string{}

	for _, event := range p.Events {
		out = append(out, map[string]string{
			"Timestamp": event.Timestamp.Format(time.RFC3339),
			"Type":      event.Type,
			"Message":   event.Message,
		})
	}

	return out
}
