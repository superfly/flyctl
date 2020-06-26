package presenters

import (
	"time"

	"github.com/superfly/flyctl/api"
)

type AllocationEvents struct {
	Events []api.AllocationEvent
}

func (p *AllocationEvents) APIStruct() interface{} {
	return p.Events
}

func (p *AllocationEvents) FieldNames() []string {
	return []string{"Timestamp", "Type", "Message"}
}

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
