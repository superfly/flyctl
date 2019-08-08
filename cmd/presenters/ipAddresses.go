package presenters

import (
	"github.com/superfly/flyctl/api"
)

type IPAddresses struct {
	IPAddresses []api.IPAddress
}

func (p *IPAddresses) FieldNames() []string {
	return []string{"Address", "Type"}
}

func (p *IPAddresses) Records() []map[string]string {
	out := []map[string]string{}

	for _, ip := range p.IPAddresses {
		out = append(out, map[string]string{
			"Address": ip.Address,
			"Type":    ip.Type,
		})
	}

	return out
}
