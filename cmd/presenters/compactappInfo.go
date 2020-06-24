package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type AppCompact struct {
	AppCompact api.AppCompact
}

func (p *AppCompact) APIStruct() interface{} {
	return p.AppCompact
}

func (p *AppCompact) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "Hostname"}
}

func (p *AppCompact) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Name":    p.AppCompact.Name,
		"Owner":   p.AppCompact.Organization.Slug,
		"Version": strconv.Itoa(p.AppCompact.Version),
		"Status":  p.AppCompact.Status,
	}

	if len(p.AppCompact.Hostname) > 0 {
		info["Hostname"] = p.AppCompact.Hostname
	} else {
		info["Hostname"] = "<empty>"
	}

	out = append(out, info)

	return out
}
