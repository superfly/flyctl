package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/superfly/flyctl/docstrings"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
)

func newDeployCommand() *Command {

	deployStrings := docstrings.Get("deploy")
	cmd := BuildCommand(nil, runDeploy, deployStrings.Usage, deployStrings.Short, deployStrings.Long, true, os.Stdout, workingDirectoryFromArg(0), requireAppName)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Shorthand:   "i",
		Description: "Image tag or id to deploy",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name: "squash",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "builder",
		Description: "Buildpack builder",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "buildpack",
		Description: "buildpack",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(ctx *CmdContext) error {
	op, err := docker.NewDeployOperation(ctx.AppName, ctx.AppConfig, ctx.FlyClient, ctx.Out, ctx.Config.GetBool("squash"))
	if err != nil {
		return err
	}

	parsedCfg, err := op.ValidateConfig()
	if err != nil {
		return err
	}

	if parsedCfg.Valid {
		printAppConfigServices("  ", *parsedCfg)
	}

	if imageRef, _ := ctx.Config.GetString("image"); imageRef != "" {
		release, err := op.DeployImage(imageRef)
		if err != nil {
			return err
		}
		return renderRelease(ctx, release)
	}

	fmt.Printf("Deploy source directory '%s'\n", ctx.WorkingDir)

	if op.DockerAvailable() {
		fmt.Println("Docker daemon available, performing local build...")

		buildpackBuilder, _ := ctx.Config.GetString("builder")
		var release *api.Release
		if buildpackBuilder != "" {
			buildpacks := ctx.Config.GetStringSlice("buildpack")
			r, err := op.PackAndDeploy(ctx.WorkingDir, ctx.AppConfig, buildpackBuilder, buildpacks)
			if err != nil {
				return err
			}
			release = r
		} else {
			r, err := op.BuildAndDeploy(ctx.WorkingDir, ctx.AppConfig)
			if err != nil {
				return err
			}
			release = r
		}

		return renderRelease(ctx, release)
	}

	fmt.Println("Docker daemon unavailable, performing remote build...")

	build, err := op.StartRemoteBuild(ctx.WorkingDir, ctx.AppConfig)
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

	return watchDeployment(ctx)
}

func renderRelease(ctx *CmdContext, release *api.Release) error {
	fmt.Printf("Release v%d created\n", release.Version)

	return watchDeployment(ctx)
}

func watchDeployment(ctx *CmdContext) error {
	if ctx.Config.GetBool("detach") {
		return nil
	}

	fmt.Println(aurora.Blue("==>"), "Monitoring Deployment")
	fmt.Println(aurora.Faint("You can detach the terminal anytime without stopping the deployment"))

	monitor := flyctl.NewDeploymentMonitor(ctx.FlyClient, ctx.AppName)
	if isatty.IsTerminal(os.Stdout.Fd()) {
		monitor.DisplayCompact(ctx.Out)
	} else {
		monitor.DisplayVerbose(ctx.Out)
	}

	if err := monitor.Error(); err != nil {
		return err
	}

	if !monitor.Success() {
		return ErrAbort
	}

	return nil
}
