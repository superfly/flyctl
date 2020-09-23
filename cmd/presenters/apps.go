package presenters

import (
	"github.com/superfly/flyctl/api"
)

type Apps struct {
	App  *api.App
	Apps []api.App
}

func (p *Apps) APIStruct() interface{} {
	return p.Apps
}

func (p *Apps) FieldNames() []string {
	return []string{"Name", "Owner", "Status", "Latest Deploy"}
}

func (p *Apps) Records() []map[string]string {
	out := []map[string]string{}

	if p.App != nil {
		p.Apps = append(p.Apps, *p.App)
	}

	for i := range p.Apps {
		latestDeploy := ""
		if p.Apps[i].Deployed && p.Apps[i].CurrentRelease != nil {
			latestDeploy = FormatRelativeTime(p.Apps[i].CurrentRelease.CreatedAt)
		}

		out = append(out, map[string]string{
			"Name":          p.Apps[i].Name,
			"Owner":         p.Apps[i].Organization.Slug,
			"Status":        p.Apps[i].Status,
			"Latest Deploy": latestDeploy,
		})
	}

	return out
}
