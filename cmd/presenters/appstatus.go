package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type AppStatus struct {
	AppStatus api.AppStatus
}

func (p *AppStatus) APIStruct() interface{} {
	return p.AppStatus
}

func (p *AppStatus) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "Hostname"}
}

func (p *AppStatus) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Name":    p.AppStatus.Name,
		"Owner":   p.AppStatus.Organization.Slug,
		"Version": strconv.Itoa(p.AppStatus.Version),
		"Status":  p.AppStatus.Status,
	}

	if len(p.AppStatus.Hostname) > 0 {
		info["Hostname"] = p.AppStatus.Hostname
	} else {
		info["Hostname"] = "<empty>"
	}

	out = append(out, info)

	return out
}
