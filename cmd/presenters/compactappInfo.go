package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type CompactAppInfo struct {
	CompactApp api.CompactApp
}

func (p *CompactAppInfo) APIStruct() interface{} {
	return p.CompactApp
}

func (p *CompactAppInfo) FieldNames() []string {
	return []string{"Name", "Owner", "Version", "Status", "Hostname"}
}

func (p *CompactAppInfo) Records() []map[string]string {
	out := []map[string]string{}

	info := map[string]string{
		"Name":    p.CompactApp.Name,
		"Owner":   p.CompactApp.Organization.Slug,
		"Version": strconv.Itoa(p.CompactApp.Version),
		"Status":  p.CompactApp.Status,
	}

	if len(p.CompactApp.Hostname) > 0 {
		info["Hostname"] = p.CompactApp.Hostname
	} else {
		info["Hostname"] = "<empty>"
	}

	out = append(out, info)

	return out
}
