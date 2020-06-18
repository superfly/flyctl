package presenters

import (
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
)

type Certificate struct {
	Certificate *api.AppCertificate
}

func (p *Certificate) APIStruct() interface{} {
	return p.Certificate
}

func (p *Certificate) FieldNames() []string {
	return []string{"Hostname", "Configured", "Issued", "Certificate Authority", "DNS Provider", "DNS Validation Instructions", "DNS Validation Hostname", "DNS Validation Target", "Source", "Created At", "Status"}
}

func (p *Certificate) Records() []map[string]string {
	out := []map[string]string{}

	types := []string{}

	for _, issued := range p.Certificate.Issued.Nodes {
		types = append(types, issued.Type)
	}

	out = append(out, map[string]string{
		"Hostname":                    p.Certificate.Hostname,
		"Configured":                  strconv.FormatBool(p.Certificate.Configured),
		"Certificate Authority":       p.Certificate.CertificateAuthority,
		"DNS Provider":                p.Certificate.DNSProvider,
		"DNS Validation Instructions": p.Certificate.DNSValidationInstructions,
		"DNS Validation Hostname":     p.Certificate.DNSValidationHostname,
		"DNS Validation Target":       p.Certificate.DNSValidationTarget,
		"Source":                      p.Certificate.Source,
		"Created At":                  formatRelativeTime(p.Certificate.CreatedAt),
		"Issued":                      strings.Join(types, ", "),
		"Status":                      p.Certificate.ClientStatus,
	})

	return out
}
