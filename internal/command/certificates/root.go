package certificates

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/dustin/go-humanize"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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

	if cert.ClientStatus == "Ready" {
		return nil
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck)
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

	if cert.ClientStatus == "Ready" {
		return nil
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck)
}

func runCertificatesAdd(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	hostname := flag.FirstArg(ctx)

	cert, hostcheck, err := apiClient.AddCertificate(ctx, appName, hostname)
	if err != nil {
		return err
	}

	return reportNextStepCert(ctx, hostname, cert, hostcheck)
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

func reportNextStepCert(ctx context.Context, hostname string, cert *fly.AppCertificate, hostcheck *fly.HostnameCheck) error {
	io := iostreams.FromContext(ctx)

	// print a blank line, easier to read!
	fmt.Fprintln(io.Out)

	colorize := io.ColorScheme()
	appName := appconfig.NameFromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)
	alternateHostname := getAlternateHostname(hostname)

	// These are the IPs we have for the app
	ips, err := apiClient.GetIPAddresses(ctx, appName)
	if err != nil {
		return err
	}

	var ipV4 fly.IPAddress
	var ipV6 fly.IPAddress
	var configuredipV4 bool
	var configuredipV6 bool

	// Extract the v4 and v6 addresses we have allocated
	for _, x := range ips {
		if x.Type == "v4" || x.Type == "shared_v4" {
			ipV4 = x
		} else if x.Type == "v6" {
			ipV6 = x
		}
	}

	// Do we have A records
	if len(hostcheck.ARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		ip := net.ParseIP(hostcheck.ARecords[0])
		if !ip.Equal(net.ParseIP(ipV4.Address)) {
			if isCloudflareProxied(cert.DNSProvider, ip) {
				return printCloudflareInstructions(ctx, hostname, cert)
			}

			fmt.Fprintf(io.Out, colorize.Yellow("A Record (%s) does not match app's IP (%s)\n"), hostcheck.ARecords[0], ipV4.Address)
		} else {
			configuredipV4 = true
		}
	}

	if len(hostcheck.AAAARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		ip := net.ParseIP(hostcheck.AAAARecords[0])
		if !ip.Equal(net.ParseIP(ipV6.Address)) {
			if isCloudflareProxied(cert.DNSProvider, ip) {
				return printCloudflareInstructions(ctx, hostname, cert)
			}
			fmt.Fprintf(io.Out, colorize.Yellow("AAAA Record (%s) does not match app's IP (%s)\n"), hostcheck.AAAARecords[0], ipV6.Address)
		} else {
			configuredipV6 = true
		}
	}

	if len(hostcheck.ResolvedAddresses) > 0 {
		for _, address := range hostcheck.ResolvedAddresses {
			ip := net.ParseIP(address)
			if ip.Equal(net.ParseIP(ipV4.Address)) {
				configuredipV4 = true
			} else if ip.Equal(net.ParseIP(ipV6.Address)) {
				configuredipV6 = true
			} else {
				if isCloudflareProxied(cert.DNSProvider, ip) {
					return printCloudflareInstructions(ctx, hostname, cert)
				}
				fmt.Fprintf(io.Out, colorize.Yellow("Address resolution (%s) does not match app's IP (%s/%s)\n"), address, ipV4.Address, ipV6.Address)
			}
		}
	}

	if cert.IsApex {
		// If this is an apex domain we should guide towards creating A and AAAA records
		addArecord := !configuredipV4
		addAAAArecord := !cert.AcmeALPNConfigured

		if addArecord || addAAAArecord {
			stepcnt := 1
			fmt.Fprintf(io.Out, "You are creating a certificate for %s\n", colorize.Bold(hostname))
			fmt.Fprintf(io.Out, "We are using %s for this certificate.\n\n", cert.CertificateAuthority)
			if addArecord {
				fmt.Fprintf(io.Out, "You can direct traffic to %s by:\n\n", hostname)
				fmt.Fprintf(io.Out, "%d: Adding an A record to your DNS service which reads\n", stepcnt)
				fmt.Fprintf(io.Out, "\n    A @ %s\n\n", ipV4.Address)
				stepcnt = stepcnt + 1
			}
			if addAAAArecord {
				fmt.Fprintf(io.Out, "You can validate your ownership of %s by:\n\n", hostname)
				fmt.Fprintf(io.Out, "%d: Adding an AAAA record to your DNS service which reads:\n\n", stepcnt)
				fmt.Fprintf(io.Out, "    AAAA @ %s\n\n", ipV6.Address)
				// stepcnt = stepcnt + 1 Uncomment if more steps
			}
		} else {
			if cert.ClientStatus == "Ready" {
				fmt.Fprintf(io.Out, "Your certificate for %s has been issued, make sure you create another certificate for %s \n", hostname, alternateHostname)
			} else {
				fmt.Fprintf(io.Out, "Your certificate for %s is being issued. Status is %s. Make sure to create another certificate for %s when the current certificate is issued. \n", hostname, cert.ClientStatus, alternateHostname)
			}
		}
	} else if cert.IsWildcard {
		// If this is an wildcard domain we should guide towards satisfying a DNS-01 challenge
		addArecord := !configuredipV4
		addCNAMErecord := !cert.AcmeDNSConfigured

		stepcnt := 1
		fmt.Fprintf(io.Out, "You are creating a wildcard certificate for %s\n", hostname)
		fmt.Fprintf(io.Out, "We are using %s for this certificate.\n\n", cert.CertificateAuthority)
		if addArecord {
			fmt.Fprintf(io.Out, "You can direct traffic to %s by:\n\n", hostname)
			fmt.Fprintf(io.Out, "%d: Adding an A record to your DNS service which reads\n", stepcnt)
			stepcnt = stepcnt + 1
			fmt.Fprintf(io.Out, "\n    A @ %s\n\n", ipV4.Address)
		}

		if addCNAMErecord {
			printDNSValidationInstructions(ctx, stepcnt, hostname, cert)
			// stepcnt = stepcnt + 1 Uncomment if more steps
		}
	} else {
		// This is not an apex domain
		// If A and AAAA record is not configured offer CNAME

		nothingConfigured := !(configuredipV4 && configuredipV6)
		onlyV4Configured := configuredipV4 && !configuredipV6

		if nothingConfigured || onlyV4Configured {
			fmt.Fprintf(io.Out, "You are creating a certificate for %s\n", hostname)
			fmt.Fprintf(io.Out, "We are using %s for this certificate.\n\n", readableCertAuthority(cert.CertificateAuthority))

			if nothingConfigured {
				fmt.Fprintf(io.Out, "You can configure your DNS for %s by:\n\n", hostname)

				eTLD, _ := publicsuffix.EffectiveTLDPlusOne(hostname)
				subdomainname := strings.TrimSuffix(hostname, eTLD)
				fmt.Fprintf(io.Out, "1: Adding an CNAME record to your DNS service which reads:\n")
				fmt.Fprintf(io.Out, "\n    CNAME %s %s.fly.dev\n", subdomainname, appName)
			} else if onlyV4Configured {
				printDNSValidationInstructions(ctx, 1, hostname, cert)
			}
		} else {
			if cert.ClientStatus == "Ready" {
				fmt.Fprintf(io.Out, "Your certificate for %s has been issued, make sure you create another certificate for %s \n", hostname, alternateHostname)
			} else {
				fmt.Fprintf(io.Out, "Your certificate for %s is being issued. Status is %s. Make sure to create another certificate for %s when the current certificate is issued. \n", hostname, cert.ClientStatus, alternateHostname)
			}
		}
	}

	return nil
}

func printDNSValidationInstructions(ctx context.Context, stepcnt int, hostname string, cert *fly.AppCertificate) {
	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "You can validate your ownership of %s by:\n\n", hostname)

	fmt.Fprintf(io.Out, "%d: Adding an CNAME record to your DNS service which reads:\n", stepcnt)
	fmt.Fprintf(io.Out, "    %s\n", cert.DNSValidationInstructions)
}

func isCloudflareProxied(provider string, ip net.IP) bool {
	if provider != CLOUDFLARE {
		return false
	}
	for _, ipnet := range CloudflareIPs {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

func printCloudflareInstructions(ctx context.Context, hostname string, cert *fly.AppCertificate) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	fmt.Fprintln(io.Out, colorize.Yellow("You're using Cloudflare's proxying feature (orange cloud active) for this hostname."))
	fmt.Fprintln(io.Out, "If you do not need Cloudflare-specific features, it's best to turn off proxying.")
	fmt.Fprintln(io.Out, "The only way to create certificates for proxied hostnames is to use the DNS challenge.")

	printDNSValidationInstructions(ctx, 1, hostname, cert)

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "If you've already set this up, your certificate should be issued soon.")
	fmt.Fprintln(io.Out, "For much more information, check our docs at: https://fly.io/docs/networking/custom-domain/")

	return nil
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
