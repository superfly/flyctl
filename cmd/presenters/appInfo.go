package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type AppInfo struct {
	App api.App
}

func (p *AppInfo) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "Hostname"}
}

func (p *AppInfo) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Name":    p.App.Name,
		"Owner":   p.App.Organization.Slug,
		"Version": strconv.Itoa(p.App.Version),
		"Status":  p.App.Status,
	}

	if len(p.App.Hostname) > 0 {
		info["Hostname"] = p.App.Hostname
	} else {
		info["Hostname"] = "<empty>"
	}

	out = append(out, info)

	return out
}
