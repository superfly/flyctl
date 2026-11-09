package certificates

import (
	"context"
	"fmt"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/net/publicsuffix"
)

// DNSRecord represents an individual DNS record that should be created.
type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Purpose string `json:"purpose,omitempty"`
}

// RoutingOption represents a routing configuration choice (e.g. A/AAAA or CNAME).
type RoutingOption struct {
	Name        string      `json:"name"`
	Recommended bool        `json:"recommended,omitempty"`
	Records     []DNSRecord `json:"records"`
}

// AdditionalRecord represents supplemental DNS records such as ACME challenge or ownership verification.
type AdditionalRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
}

// DNSOptions encapsulates computed DNS options for setting up a certificate.
type DNSOptions struct {
	Hostname          string             `json:"hostname"`
	IsApex            bool               `json:"is_apex"`
	RoutingOptions    []RoutingOption    `json:"routing_options,omitempty"`
	AdditionalRecords []AdditionalRecord `json:"additional_records,omitempty"`
}

// CertificatesSetupResponse wraps the API response and computed DNS options for JSON output.
type CertificatesSetupResponse struct {
	*fly.CertificateDetailResponse
	DNSOptions DNSOptions `json:"dns_options"`
}

func computeDNSOptions(hostname string, resp *fly.CertificateDetailResponse) DNSOptions {
	var ipV4, ipV6 string
	if len(resp.DNSRequirements.A) > 0 {
		ipV4 = resp.DNSRequirements.A[0]
	}
	if len(resp.DNSRequirements.AAAA) > 0 {
		ipV6 = resp.DNSRequirements.AAAA[0]
	}
	cnameTarget := resp.DNSRequirements.CNAME

	isWildcard := strings.HasPrefix(hostname, "*.")
	hasACME := resp.AcmeRequested
	hasCustom := false
	for _, cert := range resp.Certificates {
		if cert.Source == "custom" {
			hasCustom = true
		} else {
			hasACME = true
		}
	}
	if !hasCustom {
		hasACME = true
	}

	eTLD, _ := publicsuffix.EffectiveTLDPlusOne(hostname)
	isApex := hostname == eTLD
	showCNAME := cnameTarget != "" && !isApex

	opts := DNSOptions{
		Hostname: hostname,
		IsApex:   isApex,
	}

	hasRoutingOptions := (ipV4 != "" || ipV6 != "") || showCNAME
	if hasRoutingOptions {
		if ipV4 != "" || ipV6 != "" {
			var records []DNSRecord
			if ipV4 != "" {
				records = append(records, DNSRecord{
					Type:  "A",
					Name:  hostname,
					Value: ipV4,
				})
			}
			if ipV6 != "" {
				records = append(records, DNSRecord{
					Type:  "AAAA",
					Name:  hostname,
					Value: ipV6,
				})
			}

			opts.RoutingOptions = append(opts.RoutingOptions, RoutingOption{
				Name:        "A and AAAA Records",
				Recommended: showCNAME,
				Records:     records,
			})
		}

		if showCNAME {
			opts.RoutingOptions = append(opts.RoutingOptions, RoutingOption{
				Name:        "CNAME Record",
				Recommended: false,
				Records: []DNSRecord{
					{
						Type:  "CNAME",
						Name:  hostname,
						Value: cnameTarget,
					},
				},
			})
		}
	}

	acme := resp.DNSRequirements.ACMEChallenge
	showACME := hasACME && acme.Name != "" && acme.Target != ""

	ownership := resp.DNSRequirements.Ownership
	showOwnership := ownership.Name != "" && (!hasACME || hasCustom || !isWildcard)

	if showACME {
		purpose := "Only needed if you want to generate the certificate before directing traffic to your application."
		if isWildcard {
			purpose = "Required to issue fly-managed wildcard certificates."
		}
		opts.AdditionalRecords = append(opts.AdditionalRecords, AdditionalRecord{
			Type:    "CNAME",
			Name:    acme.Name,
			Value:   acme.Target,
			Title:   "ACME DNS Challenge",
			Purpose: purpose,
		})
	}

	if showOwnership {
		purpose := "Required if your app doesn't have an IPv6 address, or if traffic is routed through a CDN or proxy."
		if hasCustom && isWildcard {
			purpose = "Required to verify ownership for custom wildcard certificates."
		}
		opts.AdditionalRecords = append(opts.AdditionalRecords, AdditionalRecord{
			Type:    "TXT",
			Name:    ownership.Name,
			Value:   ownership.AppValue,
			Title:   "Ownership TXT Record",
			Purpose: purpose,
		})
	}

	return opts
}

func printDNSOptions(ctx context.Context, opts DNSOptions) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold(fmt.Sprintf("DNS Setup Options for %s", opts.Hostname)))

	if len(opts.RoutingOptions) > 0 {
		fmt.Fprintln(io.Out)
		if len(opts.RoutingOptions) > 1 {
			fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Route traffic to your app with one of:"))
		} else {
			fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Route traffic to your app:"))
		}
	}

	for i, opt := range opts.RoutingOptions {
		fmt.Fprintln(io.Out)
		num := i + 1
		if opt.Recommended {
			fmt.Fprintf(io.Out, "  %s\n", colorize.Green(fmt.Sprintf("%d. %s (recommended)", num, opt.Name)))
		} else if len(opts.RoutingOptions) > 1 {
			fmt.Fprintf(io.Out, "  %s\n", fmt.Sprintf("%d. %s", num, opt.Name))
		} else {
			fmt.Fprintf(io.Out, "  %s\n", colorize.Green(opt.Name))
		}

		for _, rec := range opt.Records {
			if rec.Type == "A" {
				fmt.Fprintf(io.Out, "     %s    %s \u2192 %s\n", colorize.Cyan("A"), rec.Name, rec.Value)
			} else if rec.Type == "AAAA" {
				fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan("AAAA"), rec.Name, rec.Value)
			} else {
				fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan(rec.Type), rec.Name, rec.Value)
			}
		}
	}

	if len(opts.AdditionalRecords) > 0 {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Additional DNS records:"))
	}

	for _, rec := range opts.AdditionalRecords {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "  %s\n", rec.Title)
		if rec.Value != "" {
			fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan(rec.Type), rec.Name, rec.Value)
		}
		if rec.Purpose != "" {
			fmt.Fprintf(io.Out, "     %s\n", colorize.Gray(rec.Purpose))
		}
	}

	fmt.Fprintln(io.Out)
}
