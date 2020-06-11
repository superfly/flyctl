package presenters

import (
	"github.com/superfly/flyctl/api"
)

type IPAddresses struct {
	IPAddresses []api.IPAddress
}

func (p *IPAddresses) APIStruct() interface{} {
	return nil
}

func (p *IPAddresses) FieldNames() []string {
	return []string{"Type", "Address", "Created At"}
}

func (p *IPAddresses) Records() []map[string]string {
	out := []map[string]string{}

	for _, ip := range p.IPAddresses {
		out = append(out, map[string]string{
			"Address":    ip.Address,
			"Type":       ip.Type,
			"Created At": formatRelativeTime(ip.CreatedAt),
		})
	}

	return out
}
