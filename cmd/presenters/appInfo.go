package presenters

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

type AppInfo struct {
	App api.App
}

func (p *AppInfo) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "URL"}
}

func (p *AppInfo) Records() []map[string]string {
	out := []map[string]string{}

	out = append(out, map[string]string{
		"Name":    p.App.Name,
		"Owner":   p.App.Organization.Slug,
		"Version": fmt.Sprintf("v%d", p.App.Version),
		"Status":  p.App.Status,
		"URL":     p.App.AppURL,
	})

	return out
}
