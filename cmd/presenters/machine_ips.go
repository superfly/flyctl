package presenters

import (
	"github.com/superfly/flyctl/api"
)

type MachineIPs struct {
	IPAddresses []*api.MachineIP
}

func (p *MachineIPs) APIStruct() interface{} {
	return p.IPAddresses
}

func (p *MachineIPs) FieldNames() []string {
	return []string{"Family", "Address", "Kind"}
}

func (p *MachineIPs) Records() []map[string]string {
	out := []map[string]string{}

	for _, ip := range p.IPAddresses {
		out = append(out, map[string]string{
			"Address": ip.IP,
			"Family":  ip.Family,
			"Kind":    ip.Kind,
			// "MaskSize": string(rune(ip.MaskSize)),
		})
	}

	return out
}
