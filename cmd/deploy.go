package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
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

	project, err := flyctl.LoadProject(sourceDir)
	if err != nil {
		return err
	}

	fmt.Printf("Deploy source directory '%s'\n", project.ProjectDir)

	if op.DockerAvailable() {
		fmt.Println("Docker daemon available, performing local build...")
		release, err := op.BuildAndDeploy(project)
		if err != nil {
			return err
		}
		return ctx.RenderEx(&presenters.Releases{Release: release}, presenters.Options{Vertical: true})
	}

	fmt.Println("Docker daemon unavailable, performing remote build...")

	build, err := op.StartRemoteBuild(project)
	if err != nil {
		return err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Building "
	s.Start()

	logStream := flyctl.NewBuildLogStream(build.ID, ctx.FlyClient)

	for line := range logStream.Fetch() {
		s.Stop()
		fmt.Println(line)
		s.Start()
	}

	s.FinalMSG = fmt.Sprintf("Build complete - %s\n", logStream.Status())

	s.Stop()

	if err := logStream.Err(); err != nil {
		return err
	}

	return nil
}
