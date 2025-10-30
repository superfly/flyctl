package certificates

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/dustin/go-humanize"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/certificate"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
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
		newCertificatesRemove(),
		newCertificatesShow(),
		newCertificatesCheck(),
		newCertificatesSetup(),
	)
	return cmd
}

func newCertificatesList() *cobra.Command {
	const (
		short = "List certificates for an app."
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
		short = "Add a certificate for an app."
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

func newCertificatesRemove() *cobra.Command {
	const (
		short = "Removes a certificate from an app"
		long  = `Removes a certificate from an application. Takes hostname
as a parameter to locate the certificate.`
	)
	cmd := command.New("remove <hostname>", short, long, runCertificatesRemove,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
	)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"delete"}
	return cmd
}

func newCertificatesShow() *cobra.Command {
	const (
		short = "Shows certificate information"
		long  = `Shows certificate information for an application.
Takes hostname as a parameter to locate the certificate.`
	)
	cmd := command.New("show <hostname>", short, long, runCertificatesShow,
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

func newCertificatesCheck() *cobra.Command {
	const (
		short = "Checks DNS configuration"
		long  = `Checks the DNS configuration for the specified hostname.
Displays results in the same format as the SHOW command.`
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
	apiClient := flyutil.ClientFromContext(ctx)

	certs, err := apiClient.GetAppCertificates(ctx, appName)
	if err != nil {
		return err
	}

	return printCertificates(ctx, certs)
}

func runCertificatesShow(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	cert, hostcheck, err := apiClient.CheckAppCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	printCertificate(ctx, cert)

	// Display validation errors if any exist
	if len(cert.ValidationErrors) > 0 {
		io := iostreams.FromContext(ctx)
		certificate.DisplayValidationErrors(io, cert.ValidationErrors)
	}

	if cert.ClientStatus == "Ready" {
		return nil
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck, DNSDisplaySkip)
}

func runCertificatesCheck(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	cert, hostcheck, err := apiClient.CheckAppCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	printCertificate(ctx, cert)

	// Display validation errors if any exist
	if len(cert.ValidationErrors) > 0 {
		io := iostreams.FromContext(ctx)
		certificate.DisplayValidationErrors(io, cert.ValidationErrors)
	}

	if cert.ClientStatus == "Ready" {
		return nil
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck, DNSDisplaySkip)
}

func runCertificatesAdd(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	cert, hostcheck, err := apiClient.AddCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck, DNSDisplayForce)
}

func runCertificatesRemove(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	if !flag.GetYes(ctx) {
		message := fmt.Sprintf("Remove certificate %s from app %s?", hostname, appName)

		confirm, err := prompt.Confirm(ctx, message)
		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}
	}

	cert, err := apiClient.DeleteCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Certificate %s deleted from app %s\n",
		colorize.Bold(cert.Certificate.Hostname),
		colorize.Bold(cert.App.Name),
	)

	return nil
}

func runCertificatesSetup(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	cert, hostcheck, err := apiClient.CheckAppCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck, DNSDisplayForce)
}

type DNSDisplayMode int

const (
	DNSDisplayAuto  DNSDisplayMode = iota // Show setup steps if required
	DNSDisplayForce                       // Always show setup steps
	DNSDisplaySkip                        // Never show setup steps
)

func reportNextStepCert(ctx context.Context, hostname string, cert *fly.AppCertificate, hostcheck *fly.HostnameCheck, dnsMode DNSDisplayMode) error {
	io := iostreams.FromContext(ctx)

	// print a blank line, easier to read!
	fmt.Fprintln(io.Out)

	colorize := io.ColorScheme()
	appName := appconfig.NameFromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	// These are the IPs we have for the app
	ips, err := flapsClient.GetIPAssignments(ctx, appName)
	if err != nil {
		return err
	}

	cnameTarget, err := apiClient.GetAppCNAMETarget(ctx, appName)
	if err != nil {
		return err
	}

	var ipV4 flaps.IPAssignment
	var ipV6 flaps.IPAssignment
	var configuredipV4 bool
	var configuredipV6 bool
	var externalProxyHint bool

	// Extract the v4 and v6 addresses we have allocated
	for _, x := range ips.IPs {
		switch {
		case strings.Contains(x.IP, "."):
			ipV4 = x
		case strings.Contains(x.IP, ":") && !x.IsFlycast():
			ipV6 = x
		}
	}

	// Do we have A records
	if len(hostcheck.ARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		ip := net.ParseIP(hostcheck.ARecords[0])
		if !ip.Equal(net.ParseIP(ipV4.IP)) {
			if isExternalProxied(cert.DNSProvider, ip) {
				externalProxyHint = true
			} else {
				fmt.Fprintf(io.Out, colorize.Yellow("A Record (%s) does not match app's IP (%s)\n"), hostcheck.ARecords[0], ipV4.IP)
			}
		} else {
			configuredipV4 = true
		}
	}

	if len(hostcheck.AAAARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		ip := net.ParseIP(hostcheck.AAAARecords[0])
		if !ip.Equal(net.ParseIP(ipV6.IP)) {
			if isExternalProxied(cert.DNSProvider, ip) {
				externalProxyHint = true
			} else {
				fmt.Fprintf(io.Out, colorize.Yellow("AAAA Record (%s) does not match app's IP (%s)\n"), hostcheck.AAAARecords[0], ipV6.IP)
			}
		} else {
			configuredipV6 = true
		}
	}

	if len(hostcheck.ResolvedAddresses) > 0 {
		for _, address := range hostcheck.ResolvedAddresses {
			ip := net.ParseIP(address)
			if ip.Equal(net.ParseIP(ipV4.IP)) {
				configuredipV4 = true
			} else if ip.Equal(net.ParseIP(ipV6.IP)) {
				configuredipV6 = true
			} else {
				if isExternalProxied(cert.DNSProvider, ip) {
					externalProxyHint = true
				} else {
					fmt.Fprintf(io.Out, colorize.Yellow("Address resolution (%s) does not match app's IP (%s/%s)\n"), address, ipV4.IP, ipV6.IP)
				}
			}
		}
	}

	var addDNSConfig bool
	switch {
	case cert.IsApex:
		addDNSConfig = !configuredipV4 || !configuredipV6
	case cert.IsWildcard:
		addDNSConfig = !configuredipV4 || !cert.AcmeDNSConfigured
	default:
		nothingConfigured := !(configuredipV4 && configuredipV6)
		onlyV4Configured := configuredipV4 && !configuredipV6
		addDNSConfig = nothingConfigured || onlyV4Configured
	}

	switch {
	case dnsMode == DNSDisplaySkip && addDNSConfig:
		fmt.Fprintln(io.Out, "Your DNS is not yet configured correctly.")
		fmt.Fprintf(io.Out, "Run %s to view DNS setup instructions.\n", colorize.Bold("fly certs setup "+hostname))
	case dnsMode == DNSDisplayForce || (dnsMode == DNSDisplayAuto && addDNSConfig):
		printDNSSetupOptions(DNSSetupFlags{
			Context:               ctx,
			Hostname:              hostname,
			Certificate:           cert,
			IPv4Address:           ipV4,
			IPv6Address:           ipV6,
			CNAMETarget:           cnameTarget,
			ExternalProxyDetected: externalProxyHint,
		})
	case cert.ClientStatus == "Ready":
		fmt.Fprintf(io.Out, "Your certificate for %s has been issued. \n", hostname)
	default:
		fmt.Fprintf(io.Out, "Your certificate for %s is being issued. Status is %s. \n", hostname, cert.ClientStatus)
	}

	if dnsMode != DNSDisplaySkip && !cert.IsWildcard && needsAlternateHostname(hostname) {
		alternateHostname := getAlternateHostname(hostname)
		fmt.Fprintf(io.Out, "Make sure to create another certificate for %s. \n", alternateHostname)
	}

	return nil
}

func isExternalProxied(provider string, ip net.IP) bool {
	if provider == CLOUDFLARE {
		for _, ipnet := range CloudflareIPs {
			if ipnet.Contains(ip) {
				return true
			}
		}
	} else {
		for _, ipnet := range FastlyIPs {
			if ipnet.Contains(ip) {
				return true
			}
		}
	}

	return false
}

type DNSSetupFlags struct {
	Context               context.Context
	Hostname              string
	Certificate           *fly.AppCertificate
	IPv4Address           flaps.IPAssignment
	IPv6Address           flaps.IPAssignment
	CNAMETarget           string
	ExternalProxyDetected bool
}

func printDNSSetupOptions(opts DNSSetupFlags) error {
	io := iostreams.FromContext(opts.Context)
	colorize := io.ColorScheme()
	hasIPv4 := opts.IPv4Address.IP != ""
	hasIPv6 := opts.IPv6Address.IP != ""
	promoteExtProxy := opts.ExternalProxyDetected && !opts.Certificate.IsWildcard

	fmt.Fprintf(io.Out, "You are creating a certificate for %s\n", colorize.Bold(opts.Hostname))
	fmt.Fprintf(io.Out, "We are using %s for this certificate.\n\n", readableCertAuthority(opts.Certificate.CertificateAuthority))

	if promoteExtProxy {
		fmt.Fprintln(io.Out, colorize.Blue("It looks like your hostname currently resolves to a proxy or CDN."))
		fmt.Fprintln(io.Out, "If you are planning to use a proxy or CDN in front of your Fly application,")
		fmt.Fprintf(io.Out, "using the %s will ensure Fly can generate a certificate automatically.\n", colorize.Green("external proxy setup"))
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintln(io.Out, "You can direct traffic to your Fly application by adding records to your DNS provider.")
	fmt.Fprintln(io.Out)

	fmt.Fprintln(io.Out, colorize.Bold("Choose your DNS setup:"))
	fmt.Fprintln(io.Out)

	optionNum := 1

	if promoteExtProxy {
		if hasIPv4 {
			fmt.Fprintf(io.Out, colorize.Green("%d. External proxy setup\n\n"), optionNum)
			fmt.Fprintf(io.Out, "   AAAA %s → %s\n\n", getRecordName(opts.Hostname), opts.IPv6Address.IP)
			fmt.Fprintln(io.Out, "   When proxying traffic, you should only use your application's IPv6 address.")
			fmt.Fprintln(io.Out)
			optionNum++
		} else {
			fmt.Fprintf(io.Out, colorize.Yellow("%d. External proxy setup (requires IPv6 allocation)\n"), optionNum)
			fmt.Fprintf(io.Out, "   Run: %s to allocate IPv6 address\n", colorize.Bold("fly ips allocate-v6"))
			fmt.Fprintf(io.Out, "   Then: %s to view these instructions again\n\n", colorize.Bold("fly certs setup "+opts.Hostname))
			fmt.Fprintln(io.Out, "   When proxying traffic, you should only use your application's IPv6 address.")
			fmt.Fprintln(io.Out)
			optionNum++
		}
	}

	fmt.Fprintf(io.Out, colorize.Green("%d. A and AAAA records (recommended for direct connections)\n\n"), optionNum)
	if hasIPv4 {
		fmt.Fprintf(io.Out, "   A    %s → %s\n", getRecordName(opts.Hostname), opts.IPv4Address.IP)
	} else {
		fmt.Fprintf(io.Out, "   %s\n", colorize.Yellow("No IPv4 addresses are allocated for your application."))
		fmt.Fprintf(io.Out, "   Run: %s to allocate recommended addresses\n", colorize.Bold("fly ips allocate"))
		fmt.Fprintf(io.Out, "   Then: %s to view these instructions again\n", colorize.Bold("fly certs setup "+opts.Hostname))
	}
	if hasIPv6 {
		fmt.Fprintf(io.Out, "   AAAA %s → %s\n", getRecordName(opts.Hostname), opts.IPv6Address.IP)
	} else {
		fmt.Fprintf(io.Out, "\n   %s\n", colorize.Yellow("No IPv6 addresses are allocated for your application."))
		fmt.Fprintf(io.Out, "   Run: %s to allocate a dedicated IPv6 address\n", colorize.Bold("fly ips allocate-v6"))
		fmt.Fprintf(io.Out, "   Then: %s to view these instructions again\n", colorize.Bold("fly certs setup "+opts.Hostname))
	}
	fmt.Fprintln(io.Out)
	optionNum++

	if !opts.Certificate.IsApex && (hasIPv4 || hasIPv6) && opts.CNAMETarget != "" {
		fmt.Fprintf(io.Out, colorize.Cyan("%d. CNAME record\n\n"), optionNum)
		fmt.Fprintf(io.Out, "   CNAME %s → %s\n", getRecordName(opts.Hostname), opts.CNAMETarget)
		fmt.Fprintln(io.Out)
		optionNum++
	}

	if !promoteExtProxy && !opts.Certificate.IsWildcard {
		fmt.Fprintf(io.Out, colorize.Blue("%d. External proxy setup\n\n"), optionNum)
		if hasIPv6 {
			fmt.Fprintf(io.Out, "   AAAA %s → %s\n\n", getRecordName(opts.Hostname), opts.IPv6Address.IP)
		} else {
			fmt.Fprintf(io.Out, "   %s\n", colorize.Yellow("No IPv6 addresses are allocated for your application."))
			fmt.Fprintf(io.Out, "   Run: %s to allocate a dedicated IPv6 address\n", colorize.Bold("fly ips allocate-v6"))
			fmt.Fprintf(io.Out, "   Then: %s to view these instructions again\n\n", colorize.Bold("fly certs setup "+opts.Hostname))
		}
		fmt.Fprintln(io.Out, "   Use this setup when configuring a proxy or CDN in front of your Fly application.")
		fmt.Fprintln(io.Out, "   When proxying traffic, you should only use your application's IPv6 address.")
		fmt.Fprintln(io.Out)
		// optionNum++ uncomment if steps added.
	}

	if opts.Certificate.IsWildcard {
		fmt.Fprint(io.Out, colorize.Yellow("Required: DNS Challenge\n\n"))
	} else {
		fmt.Fprint(io.Out, colorize.Yellow("Optional: DNS Challenge\n\n"))
	}
	fmt.Fprintf(io.Out, "   %s → %s\n\n", opts.Certificate.DNSValidationHostname, opts.Certificate.DNSValidationTarget)
	fmt.Fprintln(io.Out, "   Additional to one of the DNS setups.")
	if opts.Certificate.IsWildcard {
		fmt.Fprintf(io.Out, "   %s\n", colorize.Yellow("Required for this wildcard certificate."))
	} else {
		fmt.Fprintln(io.Out, "   Required for wildcard certificates, or to generate")
		fmt.Fprintln(io.Out, "   a certificate before directing traffic to your application.")
	}
	fmt.Fprintln(io.Out)

	return nil
}

func getRecordName(hostname string) string {
	eTLD, _ := publicsuffix.EffectiveTLDPlusOne(hostname)
	subdomainname := strings.TrimSuffix(hostname, eTLD)

	if subdomainname == "" {
		return "@"
	}
	return strings.TrimSuffix(subdomainname, ".")
}

func printCertificate(ctx context.Context, cert *fly.AppCertificate) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	hostname := flag.FirstArg(ctx)

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, cert)
		return
	}

	if cert.ClientStatus == "Ready" {
		fmt.Fprintf(io.Out, "The certificate for %s has been issued.\n\n", colorize.Bold(hostname))
	} else {
		fmt.Fprintf(io.Out, "The certificate for %s has not been issued yet.\n\n", colorize.Yellow(hostname))
	}

	myprnt := func(label string, value string) {
		fmt.Fprintf(io.Out, "%-25s = %s\n", label, value)
	}

	certtypes := []string{}

	for _, v := range cert.Issued.Nodes {
		certtypes = append(certtypes, v.Type)
	}

	myprnt("Hostname", cert.Hostname)
	myprnt("DNS Provider", cert.DNSProvider)
	myprnt("Certificate Authority", readableCertAuthority(cert.CertificateAuthority))
	myprnt("Issued", strings.Join(certtypes, ","))
	myprnt("Added to App", humanize.Time(cert.CreatedAt))
	myprnt("Source", cert.Source)
}

func readableCertAuthority(ca string) string {
	if ca == "lets_encrypt" {
		return "Let's Encrypt"
	}
	return ca
}

func printCertificates(ctx context.Context, certs []fly.AppCertificateCompact) error {
	io := iostreams.FromContext(ctx)

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, certs)
		return nil
	}

	fmt.Fprintf(io.Out, "%-25s %-20s %s\n", "Host Name", "Added", "Status")
	for _, v := range certs {
		fmt.Fprintf(io.Out, "%-25s %-20s %s\n", v.Hostname, humanize.Time(v.CreatedAt), v.ClientStatus)
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

func mustParseCIDR(s string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ipnet
}

const CLOUDFLARE = "cloudflare"

var CloudflareIPs = []*net.IPNet{
	mustParseCIDR("173.245.48.0/20"),
	mustParseCIDR("103.21.244.0/22"),
	mustParseCIDR("103.22.200.0/22"),
	mustParseCIDR("103.31.4.0/22"),
	mustParseCIDR("141.101.64.0/18"),
	mustParseCIDR("108.162.192.0/18"),
	mustParseCIDR("190.93.240.0/20"),
	mustParseCIDR("188.114.96.0/20"),
	mustParseCIDR("197.234.240.0/22"),
	mustParseCIDR("198.41.128.0/17"),
	mustParseCIDR("162.158.0.0/15"),
	mustParseCIDR("104.16.0.0/13"),
	mustParseCIDR("104.24.0.0/14"),
	mustParseCIDR("172.64.0.0/13"),
	mustParseCIDR("131.0.72.0/22"),
	mustParseCIDR("2400:cb00::/32"),
	mustParseCIDR("2606:4700::/32"),
	mustParseCIDR("2803:f800::/32"),
	mustParseCIDR("2405:b500::/32"),
	mustParseCIDR("2405:8100::/32"),
	mustParseCIDR("2a06:98c0::/29"),
	mustParseCIDR("2c0f:f248::/32"),
}

var FastlyIPs = []*net.IPNet{
	mustParseCIDR("23.235.32.0/20"),
	mustParseCIDR("43.249.72.0/22"),
	mustParseCIDR("103.244.50.0/24"),
	mustParseCIDR("103.245.222.0/23"),
	mustParseCIDR("103.245.224.0/24"),
	mustParseCIDR("104.156.80.0/20"),
	mustParseCIDR("140.248.64.0/18"),
	mustParseCIDR("140.248.128.0/17"),
	mustParseCIDR("146.75.0.0/17"),
	mustParseCIDR("151.101.0.0/16"),
	mustParseCIDR("157.52.64.0/18"),
	mustParseCIDR("167.82.0.0/17"),
	mustParseCIDR("167.82.128.0/20"),
	mustParseCIDR("167.82.160.0/20"),
	mustParseCIDR("167.82.224.0/20"),
	mustParseCIDR("172.111.64.0/18"),
	mustParseCIDR("185.31.16.0/22"),
	mustParseCIDR("199.27.72.0/21"),
	mustParseCIDR("199.232.0.0/16"),
	mustParseCIDR("2a04:4e40::/32"),
	mustParseCIDR("2a04:4e42::/32"),
}
