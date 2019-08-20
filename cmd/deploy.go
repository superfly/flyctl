package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docker"
)

func newDeployCommand() *Command {
	cmd := BuildCommand(nil, runDeploy, "deploy", "deploy a local image, remote image, or Dockerfile", os.Stdout, true, requireAppName)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Shorthand:   "i",
		Description: "Image tag or id to deploy",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(ctx *CmdContext) error {
	op, err := docker.NewDeployOperation(ctx.AppName(), ctx.FlyClient, ctx.Out)
	if err != nil {
		return err
	}

	if imageRef, _ := ctx.Config.GetString("image"); imageRef != "" {
		release, err := op.DeployImage(imageRef)
		if err != nil {
			return err
		}
		return ctx.RenderEx(&presenters.Releases{Release: release}, presenters.Options{Vertical: true})
	}

	sourceDir := "."

	if len(ctx.Args) > 0 {
		sourceDir = ctx.Args[0]
	}

	if op.DockerAvailable() {
		release, err := op.BuildAndDeploy(sourceDir)
		if err != nil {
			return err
		}
		return ctx.RenderEx(&presenters.Releases{Release: release}, presenters.Options{Vertical: true})
	}

	build, err := op.StartRemoteBuild(sourceDir)
	if err != nil {
		return err
	}
	fmt.Println(build, err)
	return nil
}
