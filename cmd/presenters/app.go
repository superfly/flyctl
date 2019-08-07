package presenters

import "github.com/superfly/flyctl/api"

type AppsPresenter struct {
	App  *api.App
	Apps []api.App
}

func (p *AppsPresenter) FieldNames() []string {
	return []string{"Name", "Owner"}
}

func (p *AppsPresenter) FieldMap() map[string]string {
	return map[string]string{
		"Name":  "Name",
		"Owner": "Owner",
	}
}

func (p *AppsPresenter) Records() []map[string]string {
	out := []map[string]string{}

	if p.App != nil {
		p.Apps = append(p.Apps, *p.App)
	}

	for _, app := range p.Apps {
		out = append(out, map[string]string{
			"Name":  app.Name,
			"Owner": app.Organization.Slug,
		})
	}

	return out
}
