package presenters

import (
	"github.com/superfly/flyctl/api"
)

type Certificates struct {
	Certificates []api.AppCertificate
}

func (p *Certificates) FieldNames() []string {
	return []string{"Hostname", "Created At", "Status"}
}

func (p *Certificates) Records() []map[string]string {
	out := []map[string]string{}

	for _, cert := range p.Certificates {
		out = append(out, map[string]string{
			"Hostname":   cert.Hostname,
			"Created At": formatRelativeTime(cert.CreatedAt),
			"Status":     cert.ClientStatus,
		})
	}

	return out
}
