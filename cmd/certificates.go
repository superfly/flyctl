package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"golang.org/x/net/publicsuffix"
)

func newCertificatesCommand() *Command {
	certsStrings := docstrings.Get("certs")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   certsStrings.Usage,
			Short: certsStrings.Short,
			Long:  certsStrings.Long,
		},
	}

	certsListStrings := docstrings.Get("certs.list")
	BuildCommand(cmd, runCertsList, certsListStrings.Usage, certsListStrings.Short, certsListStrings.Long, os.Stdout, requireSession, requireAppName)

	certsCreateStrings := docstrings.Get("certs.add")
	createCmd := BuildCommand(cmd, runCertAdd, certsCreateStrings.Usage, certsCreateStrings.Short, certsCreateStrings.Long, os.Stdout, requireSession, requireAppName)
	createCmd.Aliases = []string{"create"}
	createCmd.Command.Args = cobra.ExactArgs(1)

	certsDeleteStrings := docstrings.Get("certs.remove")
	deleteCmd := BuildCommand(cmd, runCertDelete, certsDeleteStrings.Usage, certsDeleteStrings.Short, certsDeleteStrings.Long, os.Stdout, requireSession, requireAppName)
	deleteCmd.Aliases = []string{"delete"}
	deleteCmd.Command.Args = cobra.ExactArgs(1)
	deleteCmd.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "accept all confirmations"})

	certsShowStrings := docstrings.Get("certs.show")
	show := BuildCommand(cmd, runCertShow, certsShowStrings.Usage, certsShowStrings.Short, certsShowStrings.Long, os.Stdout, requireSession, requireAppName)
	show.Command.Args = cobra.ExactArgs(1)

	certsCheckStrings := docstrings.Get("certs.check")
	check := BuildCommand(cmd, runCertCheck, certsCheckStrings.Usage, certsCheckStrings.Short, certsCheckStrings.Long, os.Stdout, requireSession, requireAppName)
	check.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runCertsList(commandContext *cmdctx.CmdContext) error {
	certs, err := commandContext.Client.API().GetAppCertificates(commandContext.AppName)
	if err != nil {
		return err
	}

	return printCertificates(commandContext, certs)
}

func runCertShow(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, hostcheck, err := commandContext.Client.API().CheckAppCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	if cert.ClientStatus == "Ready" {
		commandContext.Statusf("flyctl", cmdctx.STITLE, "The certificate for %s has been issued.\n\n", hostname)
		printCertificate(commandContext, cert)
		return nil
	}
	commandContext.Statusf("flyctl", cmdctx.STITLE, "The certificate for %s has not been issued yet.\n\n", hostname)
	printCertificate(commandContext, cert)
	return reportNextStepCert(commandContext, hostname, cert, hostcheck)

}

func runCertCheck(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, hostcheck, err := commandContext.Client.API().CheckAppCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	if cert.ClientStatus == "Ready" {
		// A certificate has been issued
		commandContext.Statusf("flyctl", cmdctx.SINFO, "The certificate for %s has been issued.\n", hostname)
		printCertificate(commandContext, cert)
		// Details should go here
		return nil
	}

	commandContext.Statusf("flyctl", cmdctx.SINFO, "The certificate for %s has not been issued yet.\n", hostname)

	return reportNextStepCert(commandContext, hostname, cert, hostcheck)
}

func runCertAdd(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, hostcheck, err := commandContext.Client.API().AddCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	return reportNextStepCert(commandContext, hostname, cert, hostcheck)
}

func runCertDelete(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	if !commandContext.Config.GetBool("yes") {
		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Remove certificate %s from app %s?", hostname, commandContext.AppName),
		}
		survey.AskOne(prompt, &confirm)

		if !confirm {
			return nil
		}
	}

	cert, err := commandContext.Client.API().DeleteCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	commandContext.Statusf("flyctl", cmdctx.SINFO, "Certificate %s deleted from app %s\n", cert.Certificate.Hostname, cert.App.Name)

	return nil
}

func reportNextStepCert(commandContext *cmdctx.CmdContext, hostname string, cert *api.AppCertificate, hostcheck *api.HostnameCheck) error {
	// These are the IPs we have for the app
	ips, err := commandContext.Client.API().GetIPAddresses(commandContext.AppName)
	if err != nil {
		return err
	}

	var ipV4 api.IPAddress
	var ipV6 api.IPAddress
	var configuredipV4 bool
	var configuredipV6 bool

	// Extract the v4 and v6 addresses we have allocated
	for _, x := range ips {
		if x.Type == "v4" {
			ipV4 = x
		} else if x.Type == "v6" {
			ipV6 = x
		}
	}

	// Do we have A records
	if len(hostcheck.ARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		if !net.ParseIP(hostcheck.ARecords[0]).Equal(net.ParseIP(ipV4.Address)) {
			commandContext.Statusf("flyctl", cmdctx.SWARN, "A Record (%s) does not match app's IP (%s)\n", hostcheck.ARecords[0], ipV4.Address)
		} else {
			configuredipV4 = true
		}
	}

	if len(hostcheck.AAAARecords) > 0 {
		// Let's check the first A record against our recorded addresses
		if !net.ParseIP(hostcheck.AAAARecords[0]).Equal(net.ParseIP(ipV6.Address)) {
			commandContext.Statusf("flyctl", cmdctx.SWARN, "AAAA Record (%s) does not match app's IP (%s)\n", hostcheck.AAAARecords[0], ipV6.Address)
		} else {
			configuredipV6 = true
		}
	}

	if len(hostcheck.ResolvedAddresses) > 0 {
		for _, address := range hostcheck.ResolvedAddresses {
			if net.ParseIP(address).Equal(net.ParseIP(ipV4.Address)) {
				configuredipV4 = true
			} else if net.ParseIP(address).Equal(net.ParseIP(ipV6.Address)) {
				configuredipV6 = true
			} else {
				commandContext.Statusf("flyctl", cmdctx.SWARN, "Address resolution (%s) does not match app's IP (%s/%s)\n", address, ipV4.Address, ipV6.Address)
			}
		}
	}

	if cert.IsApex {
		// If this is an apex domain we should guide towards creating A and AAAA records
		addArecord := !configuredipV4
		addAAAArecord := !cert.AcmeALPNConfigured

		if addArecord || addAAAArecord {
			stepcnt := 1
			commandContext.Statusf("flyctl", cmdctx.SINFO, "You are creating a certificate for %s\n", hostname)
			commandContext.Statusf("flyctl", cmdctx.SINFO, "We are using %s for this certificate.\n\n", cert.CertificateAuthority)
			commandContext.Statusf("flyctl", cmdctx.SINFO, "You can validate your ownership of %s by:\n\n", hostname)
			if addArecord {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "%d: Adding an A record to your DNS service which reads\n", stepcnt)
				stepcnt = stepcnt + 1
				commandContext.Statusf("flyctl", cmdctx.SINFO, "\n    A @ %s\n\n", ipV4.Address)
			}
			if addAAAArecord {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "%d: Adding an AAAA record to your DNS service which reads:\n\n", stepcnt)
				commandContext.Statusf("flyctl", cmdctx.SINFO, "    AAAA @ %s\n\n", ipV6.Address)
				commandContext.Statusf("flyctl", cmdctx.SINFO, " OR \n\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "%d: Adding an CNAME record to your DNS service which reads:\n\n", stepcnt)
				commandContext.Statusf("flyctl", cmdctx.SINFO, "    CNAME _acme-challenge %s\n", cert.DNSValidationTarget)
				stepcnt = stepcnt + 1

			}
		} else {
			if cert.ClientStatus == "Ready" {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "Your certificate for %s has been issued\n", hostname)
			} else {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "Your certificate for %s is being issued. Status is %s.\n", hostname, cert.ClientStatus)
			}
		}
	} else {
		// This is not an apex domain
		// If A and AAAA record is not configured offer CNAME

		nothingConfigured := !(configuredipV4 && configuredipV6)
		onlyV4Configured := configuredipV4 && !configuredipV6

		if nothingConfigured || onlyV4Configured {
			commandContext.Statusf("flyctl", cmdctx.SINFO, "You are creating a certificate for %s\n", hostname)
			commandContext.Statusf("flyctl", cmdctx.SINFO, "We are using %s for this certificate.\n\n", readableCertAuthority(cert.CertificateAuthority))

			commandContext.Statusf("flyctl", cmdctx.SINFO, "You can configure your DNS and validate your ownership of %s by:\n\n", hostname)

			if nothingConfigured {
				eTLD, _ := publicsuffix.EffectiveTLDPlusOne(hostname)
				subdomainname := strings.TrimSuffix(hostname, eTLD)
				commandContext.Statusf("flyctl", cmdctx.SINFO, "1: Adding an CNAME record to your DNS service which reads:\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "\n    CNAME %s %s.fly.dev\n", subdomainname, commandContext.AppName)
			} else if onlyV4Configured {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "1: Adding an CNAME record to your DNS service which reads:\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "CNAME _acme-challenge %s\n", cert.DNSValidationTarget)
			}
		} else {
			if cert.ClientStatus == "Ready" {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "Your certificate for %s has been issued\n", hostname)
			} else {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "Your certificate for %s is being issued. Status is %s.\n", hostname, cert.ClientStatus)
			}
		}
	}

	return nil
}

func printCertificate(commandContext *cmdctx.CmdContext, cert *api.AppCertificate) error {
	if commandContext.OutputJSON() {
		return commandContext.WriteJSON(cert)
	}

	myprnt := func(label string, value string) {
		commandContext.Statusf("flyctl", cmdctx.SINFO, "%-25s = %s\n\n", label, value)
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

	return nil
}

func readableCertAuthority(ca string) string {
	if ca == "lets_encrypt" {
		return "Let's Encrypt"
	}
	return ca
}

func printCertificates(commandContext *cmdctx.CmdContext, certs []api.AppCertificateCompact) error {
	if commandContext.OutputJSON() {
		return commandContext.WriteJSON(certs)
	}

	commandContext.Statusf("flyctl", cmdctx.STITLE, "%-25s %-20s %s\n", "Host Name", "Added", "Status")

	for _, v := range certs {
		commandContext.Statusf("flyctl", cmdctx.SINFO, "%-25s %-20s %s\n",
			v.Hostname,
			humanize.Time(v.CreatedAt),
			v.ClientStatus)
	}

	return nil
}
