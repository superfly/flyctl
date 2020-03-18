package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/builds"
	"github.com/superfly/flyctl/terminal"
)

func newDeployCommand() *Command {
	deployStrings := docstrings.Get("deploy")
	cmd := BuildCommand(nil, runDeploy, deployStrings.Usage, deployStrings.Short, deployStrings.Long, os.Stdout, workingDirectoryFromArg(0), requireSession, requireAppName)
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
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:   "build-only",
		Hidden: true,
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "remote-only",
		Description: "Perform builds remotely without using the local docker daemon",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(cc *CmdContext) error {
	ctx := createCancellableContext()
	op, err := docker.NewDeployOperation(ctx, cc.AppName, cc.AppConfig, cc.Client.API(), cc.Out, cc.Config.GetBool("squash"), cc.Config.GetBool("remote-only"))
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

	buildSource := docker.ImageSource(cc.WorkingDir, cc.AppConfig)
	if buildSource == docker.SourceNone {
		return docker.ErrNoDockerfile
	}

	fmt.Printf("Deploy source directory '%s'\n", cc.WorkingDir)

	var image docker.Image

	if op.DockerAvailable() {
		fmt.Println("Docker daemon available, performing local build...")

		if buildSource == docker.SourceDockerfile {
			fmt.Println("Building Dockerfile")
			if cc.AppConfig.HasBuilder() {
				terminal.Warn("Project contains both a Dockerfile and a builder, using Dockerfile")
			}

			img, err := op.BuildWithDocker(cc.WorkingDir, cc.AppConfig)
			if err != nil {
				return err
			}
			image = *img
		} else if buildSource == docker.SourceBuildpacks {
			fmt.Println("Building with buildpacks")
			img, err := op.BuildWithPack(cc.WorkingDir, cc.AppConfig)
			if err != nil {
				return err
			}
			image = *img
		}

		fmt.Printf("Image: %+v\n", image.Tag)
		fmt.Println(aurora.Bold(fmt.Sprintf("Image size: %s", humanize.Bytes(uint64(image.Size)))))

		if err := op.PushImage(image); err != nil {
			return err
		}

		if cc.Config.GetBool("build-only") {
			fmt.Printf("Image: %s\n", image.Tag)

			return nil
		}

	} else {
		fmt.Println("Docker daemon unavailable, performing remote build...")

		build, err := op.StartRemoteBuild(cc.WorkingDir, cc.AppConfig)
		if err != nil {
			return err
		}

		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = "Building "
		s.Start()

		buildMonitor := builds.NewBuildMonitor(build.ID, cc.Client.API())
		for line := range buildMonitor.Logs(ctx) {
			s.Stop()
			fmt.Println(line)
			s.Start()
		}

		s.FinalMSG = fmt.Sprintf("Build complete - %s\n", buildMonitor.Status())
		s.Stop()

		if err := buildMonitor.Err(); err != nil {
			return err
		}

		build = buildMonitor.Build()
		image = docker.Image{
			Tag: build.Image,
		}
	}

	if err := op.OptimizeImage(image); err != nil {
		return err
	}

	release, err := op.Deploy(image)
	if err != nil {
		return err
	}

	op.CleanDeploymentTags()

	return renderRelease(ctx, cc, release)
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

	monitor := flyctl.NewDeploymentMonitor(cc.Client.API(), cc.AppName)
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
