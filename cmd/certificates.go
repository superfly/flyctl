package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
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

	cert, err := commandContext.Client.API().CheckAppCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertAdd(commandContext *cmdctx.CmdContext) error {
	hostname := commandContext.Args[0]

	cert, err := commandContext.Client.API().AddCertificate(commandContext.AppName, hostname)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
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
