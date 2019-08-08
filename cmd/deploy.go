package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docker"
)

func newDeployCommand() *Command {
	cmd := BuildCommand(nil, runDeploy, "deploy", "deploy a local image, remote image, or Dockerfile", os.Stdout, true, requireAppName)

	cmd.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runDeploy(ctx *CmdContext) error {
	imageRef := ctx.Args[0]

	op, err := docker.NewDeployOperation(ctx.AppName(), ctx.FlyClient, ctx.Out)
	if err != nil {
		return err
	}

	release, err := op.Deploy(imageRef)
	if err != nil {
		return err
	}

	return ctx.RenderEx(&presenters.Releases{Release: release}, presenters.Options{Vertical: true})
}
