package presenters

import (
	"strconv"

	"github.com/superfly/flyctl/api"
)

type Certificate struct {
	Certificate *api.AppCertificate
}

func (p *Certificate) FieldNames() []string {
	return []string{"Hostname", "Configured", "Certificate Authority", "DNS Provider", "DNS Validation Target", "Source", "Requested At"}
}

func (p *Certificate) Records() []map[string]string {
	out := []map[string]string{}

	out = append(out, map[string]string{
		"Hostname":              p.Certificate.Hostname,
		"Configured":            strconv.FormatBool(p.Certificate.AcmeDNSConfigured),
		"Certificate Authority": p.Certificate.CertificateAuthority,
		"DNS Provider":          p.Certificate.DNSProvider,
		"DNS Validation Target": p.Certificate.DNSValidationTarget,
		"Source":                p.Certificate.Source,
		"Requested At":          formatRelativeTime(p.Certificate.CertificateRequestedAt),
	})

	return out
}
