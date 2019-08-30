package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newCertificatesCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "certs",
			Short: "manage certificates",
		},
	}

	BuildCommand(cmd, runCertsList, "list", "list certificates for an app", os.Stdout, true, requireAppName)
	add := BuildCommand(cmd, runCertAdd, "create <hostname>", "create a new certificate", os.Stdout, true, requireAppName)
	add.Command.Args = cobra.ExactArgs(1)
	show := BuildCommand(cmd, runCertShow, "show <hostname>", "show detailed certificate info", os.Stdout, true, requireAppName)
	show.Command.Args = cobra.ExactArgs(1)
	check := BuildCommand(cmd, runCertCheck, "check <hostname>", "check dns configuration", os.Stdout, true, requireAppName)
	check.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runCertsList(ctx *CmdContext) error {
	certs, err := ctx.FlyClient.GetAppCertificates(ctx.AppName())
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Certificates{Certificates: certs})
}

func runCertShow(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.FlyClient.GetAppCertificate(ctx.AppName(), hostname)
	if err != nil {
		return err
	}

	return ctx.RenderEx(&presenters.Certificate{Certificate: cert}, presenters.Options{Vertical: true})
}

func runCertCheck(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.FlyClient.CheckAppCertificate(ctx.AppName(), hostname)
	if err != nil {
		return err
	}

	return ctx.RenderEx(&presenters.Certificate{Certificate: cert}, presenters.Options{Vertical: true})
}

func runCertAdd(ctx *CmdContext) error {
	hostname := ctx.Args[0]

	cert, err := ctx.FlyClient.AddCertificate(ctx.AppName(), hostname)
	if err != nil {
		return err
	}

	return ctx.RenderEx(&presenters.Certificate{Certificate: cert}, presenters.Options{Vertical: true})
}
