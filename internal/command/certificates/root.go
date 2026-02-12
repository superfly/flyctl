package certificates

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
	"golang.org/x/net/publicsuffix"
)

func New() *cobra.Command {
	const (
		short = "Manage certificates"
		long  = `Manages the certificates associated with a deployed application.
Certificates are created by associating a hostname/domain with the application.
When Fly is then able to validate that hostname/domain, the platform gets
certificates issued for the hostname/domain by Let's Encrypt.`
	)
	cmd := command.New("certs", short, long, nil)
	cmd.AddCommand(
		newCertificatesList(),
		newCertificatesAdd(),
		newCertificatesImport(),
		newCertificatesRemove(),
		newCertificatesCheck(),
		newCertificatesSetup(),
	)
	return cmd
}

func newCertificatesList() *cobra.Command {
	const (
		short = "List certificates for an app"
		long  = `List the certificates associated with a deployed application.`
	)
	cmd := command.New("list", short, long, runCertificatesList,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func newCertificatesAdd() *cobra.Command {
	const (
		short = "Add a certificate for an app"
		long  = `Add a certificate for an application. Takes a hostname
as a parameter for the certificate.`
	)
	cmd := command.New("add <hostname>", short, long, runCertificatesAdd,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"create"}
	return cmd
}

func newCertificatesImport() *cobra.Command {
	const (
		short = "Import a custom certificate"
		long  = `Import a custom TLS certificate for a hostname.

Upload your own certificate and private key in PEM format. Requires domain
ownership verification via DNS before the certificate becomes active.`
	)
	cmd := command.New("import <hostname>", short, long, runCertificatesImport,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "fullchain",
			Description: "Path to certificate chain file (PEM format)",
		},
		flag.String{
			Name:        "private-key",
			Description: "Path to private key file (PEM format)",
		},
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.MarkFlagRequired("fullchain")
	cmd.MarkFlagRequired("private-key")
	return cmd
}

func runCertificatesImport(ctx context.Context) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	fullchainPath := flag.GetString(ctx, "fullchain")
	privateKeyPath := flag.GetString(ctx, "private-key")

	fullchain, err := os.ReadFile(fullchainPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate file: %w", err)
	}

	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key file: %w", err)
	}

	resp, err := flapsClient.CreateCustomCertificate(ctx, appName, fly.ImportCertificateRequest{
		Hostname:   hostname,
		Fullchain:  string(fullchain),
		PrivateKey: string(privateKey),
	})
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, resp)
		return nil
	}

	fmt.Fprintf(io.Out, "Certificate uploaded for %s\n", colorize.Bold(resp.Hostname))

	var customCert *fly.CertificateDetail
	for i := range resp.Certificates {
		if resp.Certificates[i].Source == "custom" {
			customCert = &resp.Certificates[i]
			break
		}
	}

	if customCert == nil {
		return fmt.Errorf("unexpected response: no custom certificate in response")
	}

	switch customCert.Status {
	case "pending_ownership":
		ov := resp.DNSRequirements.Ownership
		fmt.Fprintln(io.Out)
		if strings.HasPrefix(hostname, "*.") {
			fmt.Fprintf(io.Out, "%s Your custom certificate is uploaded but not yet active. Add a TXT record to verify domain ownership.\n", colorize.WarningIcon())
		} else {
			fmt.Fprintf(io.Out, "%s Your custom certificate is uploaded but not yet active. Add a TXT or AAAA record to verify domain ownership.\n", colorize.WarningIcon())
		}
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "    %s  %s  %s\n", ov.Name, colorize.Cyan("TXT"), ov.AppValue)
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "Run %s to verify.\n", colorize.Bold("fly certs check "+quoteHostname(hostname)))
	case "active":
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "%s Certificate is active!\n", colorize.SuccessIcon())
		if customCert.ExpiresAt != nil && !customCert.ExpiresAt.IsZero() {
			fmt.Fprintf(io.Out, "Expires: %s\n", humanize.Time(*customCert.ExpiresAt))
		}
	}

	return nil
}

func newCertificatesRemove() *cobra.Command {
	const (
		short = "Removes a certificate from an app"
		long  = `Removes a certificate from an application. Takes hostname
as a parameter to locate the certificate.

Use --custom to remove only the custom certificate while keeping ACME certificates.
Use --acme to stop ACME certificate issuance while keeping custom certificates.`
	)
	cmd := command.New("remove <hostname>", short, long, runCertificatesRemove,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.Bool{
			Name:        "custom",
			Description: "Remove only the custom certificate, keeping ACME certificates",
			Default:     false,
		},
		flag.Bool{
			Name:        "acme",
			Description: "Stop ACME certificate issuance, keeping custom certificates",
			Default:     false,
		},
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"delete"}
	return cmd
}

func newCertificatesCheck() *cobra.Command {
	const (
		short = "Show certificate and DNS status"
		long  = `Shows detailed certificate information and checks the DNS configuration
for the specified hostname.`
	)
	cmd := command.New("check <hostname>", short, long, runCertificatesCheck,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"show"}
	return cmd
}

func newCertificatesSetup() *cobra.Command {
	const (
		short = "Shows certificate setup instructions"
		long  = `Shows setup instructions for configuring DNS records for a certificate.
Takes hostname as a parameter to show the setup instructions for that certificate.`
	)
	cmd := command.New("setup <hostname>", short, long, runCertificatesSetup,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runCertificatesList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	resp, err := flapsClient.ListCertificates(ctx, appName, &flaps.ListCertificatesOpts{Limit: 50})
	if err != nil {
		return err
	}

	if err := printCertificates(ctx, resp.Certificates); err != nil {
		return err
	}

	if resp.NextCursor != "" {
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.Out, "\nShowing %d of %d certificates. Use the Machines API to paginate through all results.\n",
			len(resp.Certificates), resp.TotalCount)
	}

	return nil
}

func runCertificatesCheck(ctx context.Context) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	resp, err := flapsClient.CheckCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, resp)
		return nil
	}

	printCertificateDetail(ctx, resp)

	if len(resp.ValidationErrors) > 0 {
		fmt.Fprintln(io.Out)
		for _, ve := range resp.ValidationErrors {
			fmt.Fprintf(io.Out, "%s %s\n", colorize.WarningIcon(), colorize.Yellow(ve.Message))
			if ve.Remediation != "" {
				fmt.Fprintf(io.Out, "  %s\n", ve.Remediation)
			}
		}
	}

	hasActive := false
	hasPendingOwnership := false
	for _, cert := range resp.Certificates {
		if cert.Status == "active" {
			hasActive = true
		}
		if cert.Status == "pending_ownership" {
			hasPendingOwnership = true
		}
	}

	fmt.Fprintln(io.Out)

	if hasActive {
		fmt.Fprintf(io.Out, "%s %s\n", colorize.SuccessIcon(), colorize.Green("Certificate is verified and active"))
		return nil
	}

	if hasPendingOwnership {
		ownership := resp.DNSRequirements.Ownership
		if ownership.Name != "" {
			fmt.Fprintln(io.Out, "Add this DNS record to verify domain ownership:")
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "  TXT %s → %s\n", ownership.Name, ownership.AppValue)
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "Run %s after adding the record.\n", colorize.Bold("fly certs check "+quoteHostname(hostname)))
		}
		return nil
	}

	fmt.Fprintf(io.Out, "Run %s to view DNS setup instructions.\n", colorize.Bold("fly certs setup "+quoteHostname(hostname)))

	return nil
}

func runCertificatesAdd(ctx context.Context) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	resp, err := flapsClient.CreateACMECertificate(ctx, appName, fly.CreateCertificateRequest{
		Hostname: hostname,
	})
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		io := iostreams.FromContext(ctx)
		render.JSON(io.Out, resp)
		return nil
	}

	printCertAdded(ctx, hostname, resp)

	return nil
}

func quoteHostname(hostname string) string {
	if strings.Contains(hostname, "*") {
		return "'" + hostname + "'"
	}
	return hostname
}

func printCertAdded(ctx context.Context, hostname string, resp *fly.CertificateDetailResponse) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	var ipV4, ipV6 string
	if len(resp.DNSRequirements.A) > 0 {
		ipV4 = resp.DNSRequirements.A[0]
	}
	if len(resp.DNSRequirements.AAAA) > 0 {
		ipV6 = resp.DNSRequirements.AAAA[0]
	}

	isWildcard := strings.HasPrefix(hostname, "*.")
	quoted := quoteHostname(hostname)

	fmt.Fprintf(io.Out, "%s Certificate created for %s\n", colorize.SuccessIcon(), colorize.Bold(hostname))
	fmt.Fprintln(io.Out)

	if ipV4 == "" && ipV6 == "" {
		fmt.Fprintf(io.Out, "%s Your app has no public IP addresses.\n", colorize.WarningIcon())
		fmt.Fprintf(io.Out, "Run %s to allocate IPs.\n", colorize.Bold("fly ips allocate"))
	} else {
		fmt.Fprintln(io.Out, colorize.Bold("Recommended DNS setup:"))
		if ipV4 != "" {
			fmt.Fprintf(io.Out, "  %s    %s \u2192 %s\n", colorize.Cyan("A"), hostname, ipV4)
		}
		if ipV6 != "" {
			fmt.Fprintf(io.Out, "  %s %s \u2192 %s\n", colorize.Cyan("AAAA"), hostname, ipV6)
		}

		if ipV4 == "" {
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "Run %s to add an IPv4 address.\n", colorize.Bold("fly ips allocate"))
		}
	}

	if isWildcard {
		acme := resp.DNSRequirements.ACMEChallenge
		if acme.Name != "" && acme.Target != "" {
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "%s Wildcard certificates require DNS validation:\n", colorize.WarningIcon())
			fmt.Fprintf(io.Out, "  %s %s \u2192 %s\n", colorize.Cyan("CNAME"), acme.Name, acme.Target)
		}
	}

	if !isWildcard && needsAlternateHostname(hostname) {
		alternateHostname := getAlternateHostname(hostname)
		if strings.HasPrefix(alternateHostname, "www.") {
			fmt.Fprintln(io.Out)
			fmt.Fprintf(io.Out, "%s Run %s to cover the www subdomain.\n", colorize.Gray("Tip:"), colorize.Bold("fly certs add "+alternateHostname))
		}
	}

	fmt.Fprintln(io.Out)
	fmt.Fprintf(io.Out, "Run %s to check validation progress.\n", colorize.Bold("fly certs check "+quoted))
	fmt.Fprintf(io.Out, "Run %s for alternative DNS setups.\n", colorize.Bold("fly certs setup "+quoted))
}

func runCertificatesRemove(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)
	customOnly := flag.GetBool(ctx, "custom")
	acmeOnly := flag.GetBool(ctx, "acme")

	if customOnly && acmeOnly {
		return fmt.Errorf("cannot specify both --custom and --acme")
	}

	if !flag.GetYes(ctx) {
		var message string
		if customOnly {
			message = fmt.Sprintf("Remove custom certificate for %s from app %s?", hostname, appName)
		} else if acmeOnly {
			message = fmt.Sprintf("Stop ACME certificate issuance for %s on app %s?", hostname, appName)
		} else {
			message = fmt.Sprintf("Remove certificate %s from app %s?", hostname, appName)
		}

		confirm, err := prompt.Confirm(ctx, message)
		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}
	}

	var err error
	if customOnly {
		err = flapsClient.DeleteCustomCertificate(ctx, appName, hostname)
	} else if acmeOnly {
		err = flapsClient.DeleteACMECertificate(ctx, appName, hostname)
	} else {
		err = flapsClient.DeleteCertificate(ctx, appName, hostname)
	}
	if err != nil {
		return err
	}

	if customOnly {
		fmt.Fprintf(io.Out, "%s Custom certificate for %s deleted from app %s\n",
			colorize.SuccessIcon(),
			colorize.Bold(hostname),
			colorize.Bold(appName),
		)
	} else if acmeOnly {
		fmt.Fprintf(io.Out, "%s ACME certificate issuance stopped for %s on app %s\n",
			colorize.SuccessIcon(),
			colorize.Bold(hostname),
			colorize.Bold(appName),
		)
	} else {
		fmt.Fprintf(io.Out, "%s Certificate %s deleted from app %s\n",
			colorize.SuccessIcon(),
			colorize.Bold(hostname),
			colorize.Bold(appName),
		)
	}

	return nil
}

func runCertificatesSetup(ctx context.Context) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	resp, err := flapsClient.CheckCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	printDNSOptions(ctx, hostname, resp)
	return nil
}

func printDNSOptions(ctx context.Context, hostname string, resp *fly.CertificateDetailResponse) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

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

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold(fmt.Sprintf("DNS Setup Options for %s", hostname)))

	hasRoutingOptions := (ipV4 != "" || ipV6 != "") || showCNAME
	if hasRoutingOptions {
		fmt.Fprintln(io.Out)
		if showCNAME {
			fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Route traffic to your app with one of:"))
		} else {
			fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Route traffic to your app:"))
		}
	}

	optionNum := 1
	if ipV4 != "" || ipV6 != "" {
		fmt.Fprintln(io.Out)
		if showCNAME {
			fmt.Fprintf(io.Out, "  %s\n", colorize.Green(fmt.Sprintf("%d. A and AAAA Records (recommended)", optionNum)))
		} else {
			fmt.Fprintf(io.Out, "  %s\n", colorize.Green("A and AAAA Records"))
		}
		if ipV4 != "" {
			fmt.Fprintf(io.Out, "     %s    %s \u2192 %s\n", colorize.Cyan("A"), hostname, ipV4)
		}
		if ipV6 != "" {
			fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan("AAAA"), hostname, ipV6)
		}
		optionNum++
	}

	if showCNAME {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "  %s\n", fmt.Sprintf("%d. CNAME Record", optionNum))
		fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan("CNAME"), hostname, cnameTarget)
	}

	acme := resp.DNSRequirements.ACMEChallenge
	showACME := hasACME && acme.Name != "" && acme.Target != ""

	ownership := resp.DNSRequirements.Ownership
	showOwnership := ownership.Name != "" && !(hasACME && !hasCustom && isWildcard)

	if showACME || showOwnership {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Additional DNS records:"))
	}

	if showACME {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "  ACME DNS Challenge\n")
		fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan("CNAME"), acme.Name, acme.Target)
		if isWildcard {
			fmt.Fprintf(io.Out, "     %s\n", colorize.Gray("Required to issue fly-managed wildcard certificates."))
		} else {
			fmt.Fprintf(io.Out, "     %s\n", colorize.Gray("Only needed if you want to generate the certificate before directing traffic to your application."))
		}
	}

	if showOwnership {
		fmt.Fprintln(io.Out)
		fmt.Fprintf(io.Out, "  Ownership TXT Record\n")
		if ownership.AppValue != "" {
			fmt.Fprintf(io.Out, "     %s %s \u2192 %s\n", colorize.Cyan("TXT"), ownership.Name, ownership.AppValue)
		}
		if hasCustom && isWildcard {
			fmt.Fprintf(io.Out, "     %s\n", colorize.Gray("Required to verify ownership for custom wildcard certificates."))
		} else {
			fmt.Fprintf(io.Out, "     %s\n", colorize.Gray("Required if your app doesn't have an IPv6 address, or if traffic is routed through a CDN or proxy."))
		}
	}

	fmt.Fprintln(io.Out)
}

func printCertificateDetail(ctx context.Context, resp *fly.CertificateDetailResponse) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	myprnt := func(label string, value string) {
		fmt.Fprintf(io.Out, "  %s = %s\n", colorize.Gray(fmt.Sprintf("%-23s", label)), value)
	}

	var customCert, flyCert *fly.CertificateDetail
	for i := range resp.Certificates {
		if resp.Certificates[i].Source == "custom" {
			customCert = &resp.Certificates[i]
		} else {
			flyCert = &resp.Certificates[i]
		}
	}

	if customCert == nil {
		if flyCert != nil {
			printCertSection(colorize, myprnt, resp.Hostname, flyCert, "")
		} else {
			myprnt("Status", colorize.Yellow("Not verified"))
			myprnt("Hostname", resp.Hostname)
		}
		return
	}

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Custom Certificate"))
	printCertSection(colorize, myprnt, resp.Hostname, customCert, "")
	fmt.Fprintln(io.Out)

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Fly-Managed Certificate"))
	if flyCert != nil {
		flyStatus := ""
		if customCert.Status == "active" {
			flyStatus = "Fallback"
		}
		printCertSection(colorize, myprnt, resp.Hostname, flyCert, flyStatus)
	} else {
		myprnt("Status", colorize.Gray("disabled"))
	}
}

func friendlyStatus(source, rawStatus string) string {
	switch rawStatus {
	case "active":
		if source == "custom" {
			return "Verified"
		}
		return "Issued"
	case "pending_ownership":
		return "Not verified"
	default:
		if source == "custom" {
			return "Not verified"
		}
		return "Issuing..."
	}
}

func printCertSection(colorize *iostreams.ColorScheme, myprnt func(string, string), hostname string, cert *fly.CertificateDetail, statusOverride string) {
	status := friendlyStatus(cert.Source, cert.Status)
	if statusOverride != "" {
		status = statusOverride
	}

	var coloredStatus string
	switch status {
	case "Verified", "Issued":
		coloredStatus = colorize.Green(status)
	case "Issuing...":
		coloredStatus = colorize.Yellow(status)
	case "Not verified":
		coloredStatus = colorize.Yellow(status)
	default:
		coloredStatus = colorize.Gray(status)
	}
	myprnt("Status", coloredStatus)
	myprnt("Hostname", hostname)

	for _, issued := range cert.Issued {
		if issued.CertificateAuthority != "" {
			myprnt("Certificate Authority", readableCertAuthority(issued.CertificateAuthority))
			break
		}
	}

	if cert.Issuer != "" {
		myprnt("Issuer", cert.Issuer)
	}

	if len(cert.Issued) > 0 {
		var certTypes []string
		for _, issued := range cert.Issued {
			certTypes = append(certTypes, issued.Type)
		}
		myprnt("Issued", strings.Join(certTypes, ","))
	}

	if cert.CreatedAt != nil && !cert.CreatedAt.IsZero() {
		myprnt("Added to App", humanize.Time(*cert.CreatedAt))
	}

	if cert.ExpiresAt != nil && !cert.ExpiresAt.IsZero() {
		myprnt("Expires", humanize.Time(*cert.ExpiresAt))
	} else if len(cert.Issued) > 0 && !cert.Issued[0].ExpiresAt.IsZero() {
		myprnt("Expires", humanize.Time(cert.Issued[0].ExpiresAt))
	}
}

func readableCertAuthority(ca string) string {
	if ca == "lets_encrypt" {
		return "Let's Encrypt"
	}
	return ca
}

func printCertificates(ctx context.Context, certs []fly.CertificateSummary) error {
	io := iostreams.FromContext(ctx)

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, certs)
		return nil
	}

	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "%s\n", colorize.Bold(fmt.Sprintf("%-30s %-10s %s", "HOSTNAME", "SOURCE", "STATUS")))

	for _, v := range certs {
		source := "-"
		if v.HasCustomCertificate {
			source = "Custom"
		} else if v.HasFlyCertificate || v.AcmeRequested {
			source = "Fly"
		}

		var status string
		if v.HasCustomCertificate {
			if v.Configured {
				status = "Verified"
			} else {
				status = "Not verified"
			}
		} else if v.Configured {
			status = "Issued"
		} else if v.AcmeDNSConfigured || v.AcmeALPNConfigured || v.AcmeHTTPConfigured {
			status = "Issuing..."
		} else {
			status = "Not verified"
		}

		line := fmt.Sprintf("%-30s %-10s %s", v.Hostname, source, status)
		switch status {
		case "Verified", "Issued":
			line = colorize.Green(line)
		case "Not verified":
			line = colorize.Yellow(line)
		default:
			line = colorize.Yellow(line)
		}
		fmt.Fprintf(io.Out, "%s\n", line)
	}

	return nil
}

func needsAlternateHostname(hostname string) bool {
	return strings.Split(hostname, ".")[0] == "www" || len(strings.Split(hostname, ".")) == 2
}

func getAlternateHostname(hostname string) string {
	if strings.Split(hostname, ".")[0] == "www" {
		return strings.Replace(hostname, "www.", "", 1)
	} else {
		return "www." + hostname
	}
}
