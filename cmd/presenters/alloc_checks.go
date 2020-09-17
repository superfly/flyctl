package presenters

import (
	"github.com/superfly/flyctl/api"
)

// AllocationChecks - Holds check state for an allocation
type AllocationChecks struct {
	Checks []api.CheckState
}

// APIStruct - returns an interface to the check state
func (p *AllocationChecks) APIStruct() interface{} {
	return p.Checks
}

// FieldNames - returns the associated field names for check states
func (p *AllocationChecks) FieldNames() []string {
	return []string{"ID", "Service", "State", "Output"}
}

// Records - formats check states into map
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
