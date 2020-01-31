package presenters

import (
	"github.com/superfly/flyctl/api"
)

type Apps struct {
	App  *api.App
	Apps []api.App
}

func (p *Apps) FieldNames() []string {
	return []string{"Name", "Owner", "Latest Deploy"}
}

func (p *Apps) Records() []map[string]string {
	out := []map[string]string{}

	if p.App != nil {
		p.Apps = append(p.Apps, *p.App)
	}

	for _, app := range p.Apps {
		latestDeploy := ""
		if app.Deployed && app.CurrentRelease != nil {
			latestDeploy = formatRelativeTime(app.CurrentRelease.CreatedAt)
		}

		out = append(out, map[string]string{
			"Name":          app.Name,
			"Owner":         app.Organization.Slug,
			"Latest Deploy": latestDeploy,
		})
	}

	return out
}
