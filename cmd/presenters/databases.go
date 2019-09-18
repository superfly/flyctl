package presenters

import "github.com/superfly/flyctl/api"

type Databases struct {
	Database  *api.Database
	Databases []api.Database
}

func (p *Databases) FieldNames() []string {
	return []string{"ID", "Name", "Engine", "Owner"}
}

func (p *Databases) Records() []map[string]string {
	out := []map[string]string{}

	if p.Database != nil {
		p.Databases = append(p.Databases, *p.Database)
	}

	for _, db := range p.Databases {
		out = append(out, map[string]string{
			"ID":     db.ID,
			"Name":   db.Name,
			"Engine": db.Engine,
			"Owner":  db.Organization.Slug,
		})
	}

	return out
}
