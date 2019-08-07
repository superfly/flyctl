package presenters

import "github.com/superfly/flyctl/api"

type AppsPresenter struct {
	Apps []api.App
}

func (p *AppsPresenter) FieldNames() []string {
	return []string{"Name", "Owner", "Runtime"}
}

func (p *AppsPresenter) FieldMap() map[string]string {
	return map[string]string{
		"Name":    "Name",
		"Owner":   "Owner",
		"Runtime": "Runtime",
	}
}

func (p *AppsPresenter) Records() []map[string]string {
	out := []map[string]string{}

	for _, app := range p.Apps {
		out = append(out, map[string]string{
			"Name":    app.Name,
			"Owner":   app.Organization.Slug,
			"Runtime": app.Runtime,
		})
	}

	return out
}
