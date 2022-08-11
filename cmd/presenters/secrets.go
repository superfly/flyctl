package presenters

import (
	"github.com/superfly/flyctl/api"
)

type Secrets struct {
	Secrets []api.Secret
}

func (p *Secrets) APIStruct() interface{} {
	return nil
}

func (p *Secrets) FieldNames() []string {
	return []string{"Name", "Digest", "Date"}
}

func (p *Secrets) Records() []map[string]string {
	out := []map[string]string{}

	for _, secret := range p.Secrets {
		out = append(out, map[string]string{
			"Name":   secret.Name,
			"Digest": secret.Digest,
			"Date":   FormatRelativeTime(secret.CreatedAt),
		})
	}

	return out
}
