package presenters

import (
	"github.com/superfly/flyctl/api"
)

type DatabaseInfo struct {
	Database api.Database
}

func (p *DatabaseInfo) FieldNames() []string {
	return []string{"ID", "Name", "Engine", "Owner", "VM URL", "Public URL", "Created At"}
}

func (p *DatabaseInfo) Records() []map[string]string {
	out := []map[string]string{}

	out = append(out, map[string]string{
		"ID":         p.Database.BackendID,
		"Name":       p.Database.Name,
		"Engine":     p.Database.Engine,
		"Owner":      p.Database.Organization.Slug,
		"VM URL":     p.Database.VMURL,
		"Public URL": p.Database.PublicURL,
		"Created At": formatRelativeTime(p.Database.CreatedAt),
	})

	return out
}
