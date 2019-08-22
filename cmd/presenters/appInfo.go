package presenters

import (
	"github.com/superfly/flyctl/api"
)

type AppInfo struct {
	App api.App
}

func (p *AppInfo) FieldNames() []string {
	return []string{"Name", "Owner", "Status"}
}

func (p *AppInfo) Records() []map[string]string {
	out := []map[string]string{}

	out = append(out, map[string]string{
		"Name":   p.App.Name,
		"Owner":  p.App.Organization.Slug,
		"Status": p.App.Status,
	})

	return out
}
