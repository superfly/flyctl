package presenters

import (
	"github.com/superfly/flyctl/api"
)

type AllocationChecks struct {
	Checks []api.CheckState
}

func (p *AllocationChecks) APIStruct() interface{} {
	return p.Checks
}

func (p *AllocationChecks) FieldNames() []string {
	return []string{"ID", "Service", "State", "Output"}
}

func (p *AllocationChecks) Records() []map[string]string {
	out := []map[string]string{}

	for _, check := range p.Checks {
		out = append(out, map[string]string{
			"ID":      check.Name,
			"Service": check.ServiceName,
			"State":   check.Status,
			"Output":  check.Output,
		})
	}

	return out
}
