package presenters

import "github.com/superfly/flyctl/api"

type Builds struct {
	Builds []api.Build
}

func (p *Builds) APIStruct() interface{} {
	return p.Builds
}

func (p *Builds) FieldNames() []string {
	return []string{"ID", "Status", "User", "Created At", "Updated At"}
}

func (p *Builds) Records() []map[string]string {
	out := []map[string]string{}

	for _, build := range p.Builds {
		out = append(out, map[string]string{
			"ID":         build.ID,
			"Status":     build.Status,
			"User":       build.User.Email,
			"Created At": FormatRelativeTime(build.CreatedAt),
			"Updated At": FormatRelativeTime(build.UpdatedAt),
		})
	}

	return out
}
