package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type AppInfo struct {
	AppInfo api.AppInfo
}

func (p *AppInfo) APIStruct() interface{} {
	return p.AppInfo
}

func (p *AppInfo) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "Hostname"}
}

func (p *AppInfo) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Name":    p.AppInfo.Name,
		"Owner":   p.AppInfo.Organization.Slug,
		"Version": strconv.Itoa(p.AppInfo.Version),
		"Status":  p.AppInfo.Status,
	}

	if len(p.AppInfo.Hostname) > 0 {
		info["Hostname"] = p.AppInfo.Hostname
	} else {
		info["Hostname"] = "<empty>"
	}

	out = append(out, info)

	return out
}
