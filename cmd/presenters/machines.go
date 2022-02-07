package presenters

import (
	"github.com/dustin/go-humanize"
	"github.com/superfly/flyctl/api"
)

type Machines struct {
	Machine  *api.Machine
	Machines []api.Machine
}

func (p *Machines) APIStruct() interface{} {
	return p.Machines
}

func (p *Machines) FieldNames() []string {
	return []string{"ID", "Name", "State", "Region", "Created", "Hostname"}
}

func (p *Machines) Records() []map[string]string {
	out := []map[string]string{}

	if p.Machine != nil {
		p.Machines = append(p.Machines, *p.Machine)
	}

	for i := range p.Machines {

		out = append(out, map[string]string{
			"ID":       p.Machines[i].ID,
			"Name":     p.Machines[i].Name,
			"State":    p.Machines[i].State,
			"Region":   p.Machines[i].Region,
			"Created":  humanize.Time(p.Machines[i].CreatedAt),
			"Hostname": p.Machines[i].App.Hostname,
		})
	}

	return out
}
