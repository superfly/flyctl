package cmd

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
)

func newDeployCommand() *Command {
	cmd := BuildCommand(nil, runDeploy, "deploy", "deploy a local image, remote image, or Dockerfile", os.Stdout, true, requireAppName, loadProjectFromPathInFirstArg)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Shorthand:   "i",
		Description: "Image tag or id to deploy",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(ctx *CmdContext) error {
	op, err := docker.NewDeployOperation(ctx.AppName(), ctx.Project, ctx.FlyClient, ctx.Out)
	if err != nil {
		return err
	}

	if imageRef, _ := ctx.Config.GetString("image"); imageRef != "" {
		release, err := op.DeployImage(imageRef)
		if err != nil {
			return err
		}
		return renderRelease(ctx, release)
	}

	sourceDir := "."

	if len(ctx.Args) > 0 {
		sourceDir = ctx.Args[0]
	}

	project, err := flyctl.LoadProject(sourceDir)
	if err != nil {
		return err
	}
	if project.ConfigFileLoaded() {
		fmt.Printf("App config file '%s'\n", project.ConfigFilePath())
	}

	fmt.Printf("Deploy source directory '%s'\n", project.ProjectDir)

	if op.DockerAvailable() {
		fmt.Println("Docker daemon available, performing local build...")
		release, err := op.BuildAndDeploy(project)
		if err != nil {
			return err
		}

		return renderRelease(ctx, release)
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

	return watchDeployment(ctx)
}

func watchBuildLogs(ctx *CmdContext, build *api.Build) {

}

func renderRelease(ctx *CmdContext, release *api.Release) error {
	fmt.Printf("Release v%d created\n", release.Version)

	return watchDeployment(ctx)
}

func watchDeployment(ctx *CmdContext) error {
	if ctx.Config.GetBool("detach") {
		return nil
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return nil
	}

	fmt.Println(aurora.Blue("==>"), "Monitoring Deployment")
	fmt.Println(aurora.Faint("You can detach the terminal anytime without stopping the deployment"))
	fmt.Println()

	var previousRelease *api.Release
	var currentRelease *api.Release

	var app *api.App
	var err error

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Prefix = "Deploying "
	s.Start()

	if runtime.GOOS != "windows" {
		ctx.Terminal.HideCursor()
		defer ctx.Terminal.ShowCursor()
	}

	for {
		previousRelease = currentRelease
		if currentRelease != nil && currentRelease.InProgress {
			release, err := ctx.FlyClient.GetAppReleaseVersion(ctx.AppName(), currentRelease.Version)
			if err != nil {
				fmt.Println(err)
			}
			currentRelease = release
		} else {
			app, err = ctx.FlyClient.GetAppStatus(ctx.AppName(), false)
			if err != nil {
				return err
			}
			currentRelease = app.CurrentRelease
		}

		s.Lock()

		// move to the start of the column to overwrite the status indicator
		ctx.Terminal.Column(0)

		if previousRelease != nil && previousRelease.Version != currentRelease.Version {
			ctx.Terminal.ResetPosition()
		} else {
			ctx.Terminal.Overwrite()
		}

		err = ctx.RenderView(PresenterOption{
			Presentable: &presenters.ReleaseDetails{Release: *currentRelease},
			Vertical:    true,
		},
			PresenterOption{
				Presentable: &presenters.DeploymentTaskStatus{Release: *currentRelease},
			},
		)

		s.Unlock()

		if !currentRelease.InProgress && currentRelease.Stable {
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	s.Stop()

	fmt.Println(aurora.Bold("App"))
	err = ctx.RenderEx(&presenters.AppInfo{App: *app}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	return nil
}
