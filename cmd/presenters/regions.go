package presenters

import (
	"github.com/superfly/flyctl/api"
)

type Regions struct {
	Regions []api.Region
}

func (p *Regions) APIStruct() interface{} {
	return p.Regions
}

func (p *Regions) FieldNames() []string {
	return []string{"Code", "Name"}
}

func (p *Regions) Records() []map[string]string {
	out := []map[string]string{}

	for _, region := range p.Regions {
		out = append(out, map[string]string{
			"Code": region.Code,
			"Name": region.Name,
		})
	}

	return out
}
