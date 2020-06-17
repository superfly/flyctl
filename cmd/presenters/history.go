package presenters

import (
	"github.com/superfly/flyctl/api"
)

type AppHistory struct {
	AppChanges []api.AppChange
}

func (p *AppHistory) APIStruct() interface{} {
	return p.AppChanges
}

func (p *AppHistory) FieldNames() []string {
	return []string{"Type", "Status", "Description", "User", "Date"}
}

func (p *AppHistory) Records() []map[string]string {
	out := []map[string]string{}

	for _, change := range p.AppChanges {
		out = append(out, map[string]string{
			"Type":        change.Actor.Type,
			"Status":      change.Status,
			"Description": change.Description,
			"User":        change.User.Email,
			"Date":        formatRelativeTime(change.CreatedAt),
		})
	}

	return out
}
