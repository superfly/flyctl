package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/sourcecode"

	"github.com/superfly/flyctl/docstrings"
)

func newLaunchCommand() *Command {
	launchStrings := docstrings.Get("launch")
	launchCmd := BuildCommandKS(nil, runLaunch, launchStrings, os.Stdout, requireSession)
	launchCmd.Args = cobra.NoArgs
	launchCmd.AddStringFlag(StringFlagOpts{Name: "path", Description: `path to app code and where a fly.toml file will be saved.`, Default: "."})
	launchCmd.AddStringFlag(StringFlagOpts{Name: "org", Description: `the organization that will own the app`})
	launchCmd.AddStringFlag(StringFlagOpts{Name: "name", Description: "the name of the new app"})
	launchCmd.AddStringFlag(StringFlagOpts{Name: "region", Description: "the region to launch the new app in"})
	launchCmd.AddStringFlag(StringFlagOpts{Name: "image", Description: "the image to launch"})

	return launchCmd
}

func runLaunch(cmdctx *cmdctx.CmdContext) error {
	dir, _ := cmdctx.Config.GetString("path")

	if absDir, err := filepath.Abs(dir); err == nil {
		dir = absDir
	}
	cmdctx.WorkingDir = dir

	fmt.Println("Creating app in", dir)

	appConfig := flyctl.NewAppConfig()

	var srcInfo *sourcecode.SourceInfo

	configFilePath := filepath.Join(dir, "fly.toml")

	if exists, _ := flyctl.ConfigFileExistsAtPath(configFilePath); exists {
		cfg, err := flyctl.LoadAppConfig(configFilePath)
		if err != nil {
			return err
		}
		if cfg.AppName != "" {
			fmt.Println("An existing fly.toml file was found for app", cfg.AppName)
		} else {
			fmt.Println("An existing fly.toml file was found")
		}
		if confirm("Would you like to copy it's configuration to the new app?") {
			appConfig.Definition = cfg.Definition
		}
	}

	if img, _ := cmdctx.Config.GetString("image"); img != "" {
		fmt.Println("Using image", img)
		appConfig.Build = &flyctl.Build{
			Image: img,
		}
	} else {
		fmt.Println("Scanning source code")

		if si, err := sourcecode.Scan(dir); err != nil {
			return err
		} else {
			srcInfo = si
		}

		if srcInfo == nil {
			fmt.Println("Could not find a Dockerfile or detect a buildpack from source code. Continuing with a blank app.")
		} else {
			fmt.Printf("Detected %s app\n", srcInfo.Family)

			if srcInfo.Builder != "" {
				fmt.Println("Using the following build configuration:")
				fmt.Println("\tBuilder:", srcInfo.Builder)
				fmt.Println("\tBuildpacks:", strings.Join(srcInfo.Buildpacks, " "))

				appConfig.Build = &flyctl.Build{
					Builder:    srcInfo.Builder,
					Buildpacks: srcInfo.Buildpacks,
				}
			}
		}
	}

	appName := ""
	if name, _ := cmdctx.Config.GetString("name"); name != "" {
		appName = name
	}

	orgSlug, _ := cmdctx.Config.GetString("org")
	org, err := selectOrganization(cmdctx.Client.API(), orgSlug)
	if err != nil {
		return err
	}

	regionCode, _ := cmdctx.Config.GetString("region")
	region, err := selectRegion(cmdctx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	app, err := cmdctx.Client.API().CreateApp(appName, org.ID, &region.Code)
	if err != nil {
		return err
	}
	appConfig.Definition = app.Config.Definition

	cmdctx.AppName = app.Name
	appConfig.AppName = app.Name
	cmdctx.AppConfig = appConfig

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
