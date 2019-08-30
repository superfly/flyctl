package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type Certificates struct {
	Certificates []api.AppCertificate
}

func (p *Certificates) FieldNames() []string {
	return []string{"Hostname", "Configured", "Requested At"}
}

func (p *Certificates) Records() []map[string]string {
	out := []map[string]string{}

	for _, cert := range p.Certificates {
		out = append(out, map[string]string{
			"Hostname":     cert.Hostname,
			"Configured":   strconv.FormatBool(cert.AcmeDNSConfigured),
			"Requested At": formatRelativeTime(cert.CertificateRequestedAt),
		})
	}

	return out
}
