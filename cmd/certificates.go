package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
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

func runCertsList(ctx *CmdContext) error {
	certs, err := ctx.Client.API().GetAppCertificates(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Certificates{Certificates: certs})
}

func runCertShow(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.Client.API().GetAppCertificate(ctx.AppName, hostname)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertCheck(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.Client.API().CheckAppCertificate(ctx.AppName, hostname)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertAdd(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.Client.API().AddCertificate(ctx.AppName, hostname)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, PresenterOption{Presentable: &presenters.Certificate{Certificate: cert}, Vertical: true})
}

func runCertDelete(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	if !ctx.Config.GetBool("yes") {
		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Remove certificate %s from app %s?", hostname, ctx.AppName),
		}
		survey.AskOne(prompt, &confirm)

		if !confirm {
			return nil
		}
	}

	cert, err := ctx.Client.API().DeleteCertificate(ctx.AppName, hostname)
	if err != nil {
		return err
	}

	fmt.Printf("Certificate %s deleted from app %s\n", aurora.Bold(cert.Certificate.Hostname), aurora.Bold(cert.App.Name))

	return nil
}
