package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type AutoscalingRegionConfigs struct {
	Regions []api.AutoscalingRegionConfig
}

func (p *AutoscalingRegionConfigs) APIStruct() interface{} {
	return nil
}

func (p *AutoscalingRegionConfigs) FieldNames() []string {
	return []string{"Region", "Min Count", "Weight"}
}

func (p *AutoscalingRegionConfigs) Records() []map[string]string {
	out := []map[string]string{}

	for _, region := range p.Regions {
		out = append(out, map[string]string{
			"Region":    region.Code,
			"Min Count": strconv.Itoa(region.MinCount),
			"Weight":    strconv.Itoa(region.Weight),
		})
	}

	return out
}
