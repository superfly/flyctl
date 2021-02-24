package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/sourcecode"

	"github.com/superfly/flyctl/docstrings"
)

func newLaunchCommand() *Command {
	launchStrings := docstrings.Get("launch")
	launchCmd := BuildCommandKS(nil, runLaunch, launchStrings, os.Stdout, requireSession)
	launchCmd.Hidden = true
	launchCmd.Args = cobra.MaximumNArgs(1)

	return launchCmd
}

func runLaunch(cmdctx *cmdctx.CmdContext) error {
	dir := "."
	if len(cmdctx.Args) > 0 {
		dir = cmdctx.Args[0]
	}

	appConfig := flyctl.NewAppConfig()

	fmt.Println("scanning source", dir)

	srcInfo, err := sourcecode.Scan(dir)

	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", srcInfo)

	if srcInfo == nil {
		fmt.Println("Could not find a Dockerfile or detect a buildpack from source code. See the docs for help (add link here!). Continuing with a blank app.")
	} else {
		fmt.Printf("Detected %s app\n", srcInfo.Family)

		if len(srcInfo.Buildpacks) > 0 {
			appConfig.Build = &flyctl.Build{
				Builder:    srcInfo.Builder,
				Buildpacks: srcInfo.Buildpacks,
			}
		}
	}

	appName, err := inputAppName(sourcecode.SuggestAppName(dir))
	if err != nil {
		return err
	}
	cmdctx.AppName = appName
	appConfig.AppName = appName
	cmdctx.AppConfig = appConfig
	cmdctx.WorkingDir = dir

	org, err := selectOrganization(cmdctx.Client.API(), "")
	if err != nil {
		return err
	}

	app, err := cmdctx.Client.API().CreateApp(appName, org.ID)
	appConfig.Definition = app.Config.Definition

	if srcInfo != nil && (len(srcInfo.Buildpacks) > 0 || srcInfo.Builder != "") {
		appConfig.SetInternalPort(8080)
		appConfig.SetEnvVariable("PORT", "8080")
	}

	fmt.Printf("Created app %s in organization %s\n", app.Name, org.Slug)

	if err := writeAppConfig(filepath.Join(dir, "fly.toml"), appConfig); err != nil {
		return err
	}

	if srcInfo == nil {
		return nil
	}

	if !confirm("Would you like to deploy now?") {
		return nil
	}

	return runDeploy(cmdctx)

	// return nil

	// if srcInfo != nil {
	// 	var img *docker.Image
	// 	ctx := createCancellableContext()
	// 	cmdctx.AppConfig = flyctl.NewAppConfig()
	// 	cmdctx.AppName = srcInfo.Name
	// 	cmdctx.WorkingDir = dir
	// 	buildOp, err := docker.NewBuildOperation(ctx, cmdctx)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	if srcInfo.DockerfilePath != "" {
	// 		fmt.Println("Dockerfile detected, attempting to build")

	// 		img, err = buildOp.BuildWithDocker(cmdctx, srcInfo.DockerfilePath, nil)
	// 	} else if len(srcInfo.Buildpacks) > 0 {
	// 		fmt.Println("Buildpacks detected, attempting to build")

	// 		cmdctx.AppConfig.Build = &flyctl.Build{
	// 			Builder:    srcInfo.Builder,
	// 			Buildpacks: srcInfo.Buildpacks,
	// 		}

	// 		img, err = buildOp.BuildWithPack(cmdctx, nil)
	// 	}

	// 	if err != nil {
	// 		return err
	// 	}

	// 	fmt.Printf("build succeeded: %+v\n", img)
	// }

	// return nil
}
