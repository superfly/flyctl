package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
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

	certsCreateStrings := docstrings.Get("certs.create")
	create := BuildCommand(cmd, runCertAdd, certsCreateStrings.Usage, certsCreateStrings.Short, certsCreateStrings.Long, os.Stdout, requireSession, requireAppName)
	create.Command.Args = cobra.ExactArgs(1)

	certsDeleteStrings := docstrings.Get("certs.delete")
	deleteCmd := BuildCommand(cmd, runCertDelete, certsDeleteStrings.Usage, certsDeleteStrings.Short, certsDeleteStrings.Long, os.Stdout, requireSession, requireAppName)
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

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificates{Certificates: certs}})
}

func runCertShow(commmandContext *cmdctx.CmdContext) error {
	hostname := commmandContext.Args[0]

	cert, err := commmandContext.Client.API().GetAppCertificate(commmandContext.AppName, hostname)
	if err != nil {
		return err
	}

	return commmandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertCheck(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, hostcheck, err := commandContext.Client.API().CheckAppCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}
	commandContext.WriteJSON(hostcheck)

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertAdd(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, hostcheck, err := commandContext.Client.API().AddCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	//commandContext.WriteJSON(hostcheck)
	// // These will be the DNS IPs
	iprecords, _ := net.LookupIP(hostname)

	//fmt.Println(iprecords)

	// These are the IPs we have for the app
	ips, err := commandContext.Client.API().GetIPAddresses(commandContext.AppName)
	if err != nil {
		return err
	}

	var ipV4 api.IPAddress
	var ipV6 api.IPAddress
	var configuredipV4 bool
	var configuredipV6 bool

	for _, x := range ips {
		if x.Type == "v4" {
			ipV4 = x
			// Now we search the ipRecords to find a match
			for _, c := range iprecords {
				if c.String() == x.Address {
					configuredipV4 = true
					break
				}
			}
		}
		if x.Type == "v6" {
			ipV6 = x
			for _, c := range iprecords {
				if c.String() == x.Address {
					configuredipV6 = true
					break
				}
			}
		}
	}

	// fmt.Println("v4", configuredipV4, "v6", configuredipV6)

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
				commandContext.Statusf("flyctl", cmdctx.SINFO, "%d: Adding an AAAA record to your DNS service which reads:\n", stepcnt)
				stepcnt = stepcnt + 1
				commandContext.Statusf("flyctl", cmdctx.SINFO, "\n    AAAA @ %s\n\n", ipV6.Address)
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
			commandContext.Statusf("flyctl", cmdctx.SINFO, "You can configure your DNS and validate your ownership of %s by:\n\n", hostname)

			if nothingConfigured {
				eTLD, _ := publicsuffix.PublicSuffix(hostname)
				subdomainname := strings.TrimSuffix(hostname, eTLD)
				commandContext.Statusf("flyctl", cmdctx.SINFO, "1: Adding an CNAME record to your DNS service which reads:\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "\n    CNAME %s %s.fly.dev\n", subdomainname, commandContext.AppName)
			} else if onlyV4Configured {
				commandContext.Statusf("flyctl", cmdctx.SINFO, "1: Adding an CNAME record to your DNS service which reads:\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "CNAME acme-challenge %s\n", cert.DNSValidationTarget)

				commandContext.Statusf("flyctl", cmdctx.SINFO, "2: Adding an CNAME record to your DNS service which reads:\n")
				commandContext.Statusf("flyctl", cmdctx.SINFO, "CNAME _acme-challenge %s\n", cert.DNSValidationTarget)
			}
		} else {
			commandContext.Statusf("flyctl", cmdctx.SINFO, "Awaiting text\n")
		}
	}

	//return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})

	return nil
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
