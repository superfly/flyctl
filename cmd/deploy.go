package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/terminal"

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
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name: "squash",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(cc *CmdContext) error {
	ctx := createCancellableContext()
	op, err := docker.NewDeployOperation(ctx, cc.AppName, cc.AppConfig, cc.FlyClient, cc.Out, cc.Config.GetBool("squash"))
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

	if imageRef, _ := cc.Config.GetString("image"); imageRef != "" {
		release, err := op.DeployImage(imageRef)
		if err != nil {
			return err
		}
		return renderRelease(ctx, cc, release)
	}

	fmt.Printf("Deploy source directory '%s'\n", cc.WorkingDir)

	if op.DockerAvailable() {
		fmt.Println("Docker daemon available, performing local build...")

		var release *api.Release
		if op.HasDockerfile(cc.WorkingDir) {
			if cc.AppConfig.HasBuilder() {
				terminal.Warn("Project contains both a Dockerfile and a builder, using Dockerfile")
			}
			r, err := op.BuildAndDeploy(cc.WorkingDir, cc.AppConfig)
			if err != nil {
				return err
			}
			release = r
		} else if cc.AppConfig.HasBuilder() {
			r, err := op.PackAndDeploy(cc.WorkingDir, cc.AppConfig)
			if err != nil {
				return err
			}
			release = r
		} else {
			return docker.ErrNoDockerfile
		}

		return renderRelease(ctx, cc, release)
	}

	fmt.Println("Docker daemon unavailable, performing remote build...")

	build, err := op.StartRemoteBuild(cc.WorkingDir, cc.AppConfig)
	if err != nil {
		return err
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Building "
	s.Start()

	logStream := flyctl.NewBuildLogStream(build.ID, cc.FlyClient)

	defer func() {
		s.FinalMSG = fmt.Sprintf("Build complete - %s\n", logStream.Status())
		s.Stop()
	}()

	for line := range logStream.Fetch(ctx) {
		s.Stop()
		fmt.Println(line)
		s.Start()
	}

	if err := logStream.Err(); err != nil {
		return err
	}

	return watchDeployment(ctx, cc)
}

func renderRelease(ctx context.Context, cc *CmdContext, release *api.Release) error {
	fmt.Printf("Release v%d created\n", release.Version)

	return watchDeployment(ctx, cc)
}

func watchDeployment(ctx context.Context, cc *CmdContext) error {
	if cc.Config.GetBool("detach") {
		return nil
	}

	fmt.Println(aurora.Blue("==>"), "Monitoring Deployment")
	fmt.Println(aurora.Faint("You can detach the terminal anytime without stopping the deployment"))

	monitor := flyctl.NewDeploymentMonitor(cc.FlyClient, cc.AppName)
	if isatty.IsTerminal(os.Stdout.Fd()) {
		monitor.DisplayCompact(ctx, cc.Out)
	} else {
		monitor.DisplayVerbose(ctx, cc.Out)
	}

	if err := monitor.Error(); err != nil {
		return err
	}

	if !monitor.Success() {
		return ErrAbort
	}

	return nil
}
