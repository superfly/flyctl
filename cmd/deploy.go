package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/deployment"
)

func newDeployCommand(client *client.Client) *Command {
	deployStrings := docstrings.Get("deploy")
	cmd := BuildCommandKS(nil, runDeploy, deployStrings, client, workingDirectoryFromArg(0), requireSession, requireAppName)
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
		Name:   "build-only",
		Hidden: true,
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "remote-only",
		Description: "Perform builds remotely without using the local docker daemon",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "local-only",
		Description: "Only perform builds locally using the local docker daemon",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "strategy",
		Description: "The strategy for replacing running instances. Options are canary, rolling, bluegreen, or immediate. Default is canary",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "dockerfile",
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "build-arg",
		Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image-label",
		Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	cmdCtx.Status("deploy", cmdctx.STITLE, "Deploying", cmdCtx.AppName)

	cmdCtx.Status("deploy", cmdctx.SBEGIN, "Validating App Configuration")

	if cmdCtx.AppConfig == nil {
		cmdCtx.AppConfig = flyctl.NewAppConfig()
	}

	if extraEnv := cmdCtx.Config.GetStringSlice("env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("env"))
		if err != nil {
			return errors.Wrap(err, "invalid env")
		}
		cmdCtx.AppConfig.SetEnvVariables(parsedEnv)
	}

	parsedCfg, err := cmdCtx.Client.API().ParseConfig(cmdCtx.AppName, cmdCtx.AppConfig.Definition)
	if err != nil {
		if parsedCfg == nil {
			// No error data has been returned
			return fmt.Errorf("not possible to validate configuration: server returned %s", err)
		}
		for _, error := range parsedCfg.Errors {
			//	fmt.Println("   ", aurora.Red("✘").String(), error)
			cmdCtx.Status("deploy", cmdctx.SERROR, "   ", aurora.Red("✘").String(), error)
		}
		return err
	}
	cmdCtx.AppConfig.Definition = parsedCfg.Definition
	cmdCtx.Status("deploy", cmdctx.SDONE, "Validating App Configuration done")

	if parsedCfg.Valid && len(parsedCfg.Services) > 0 {
		cmdfmt.PrintServicesList(cmdCtx.IO, parsedCfg.Services)
	}

	var daemonType imgsrc.DockerDaemonType
	if !cmdCtx.Config.GetBool("local-only") {
		daemonType = daemonType | imgsrc.DockerDaemonTypeLocal
	}
	if !cmdCtx.Config.GetBool("remote-only") {
		daemonType = daemonType | imgsrc.DockerDaemonTypeRemote
	}

	resolver := imgsrc.NewResolver(daemonType, cmdCtx.Client.API(), cmdCtx.AppName)

	opts := imgsrc.ImageOptions{
		AppName:    cmdCtx.AppName,
		WorkingDir: cmdCtx.WorkingDir,
		AppConfig:  cmdCtx.AppConfig,
	}
	opts.ImageRef, _ = cmdCtx.Config.GetString("image")
	opts.ImageLabel, _ = cmdCtx.Config.GetString("image-label")
	opts.DockerfilePath, _ = cmdCtx.Config.GetString("dockerfile")
	extraArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
	if err != nil {
		return errors.Wrap(err, "invalid build-arg")
	}
	opts.ExtraBuildArgs = extraArgs

	img, err := resolver.Resolve(ctx, cmdCtx.IO, opts)
	if err != nil {
		return err
	}
	if img == nil {
		return errors.New("could not find an image to deploy")
	}

	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image: %+v\n", img.Tag)
	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	if cmdCtx.Config.GetBool("build-only") {
		return nil
	}

	cmdCtx.Status("deploy", cmdctx.SBEGIN, "Creating Release")

	input := api.DeployImageInput{
		AppID: cmdCtx.AppName,
		Image: img.Tag,
	}
	if val, _ := cmdCtx.Config.GetString("strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ToUpper(val))
	}
	if cmdCtx.AppConfig != nil && len(cmdCtx.AppConfig.Definition) > 0 {
		input.Definition = api.DefinitionPtr(cmdCtx.AppConfig.Definition)
	}

	release, err := cmdCtx.Client.API().DeployImage(input)
	if err != nil {
		return err
	}

	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Release v%d created\n", release.Version)
	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Deploying to %s.fly.dev\n", cmdCtx.AppName)

	if release.DeploymentStrategy == "IMMEDIATE" {
		return nil
	}

	if cmdCtx.Config.GetBool("detach") {
		return nil
	}

	return watchDeployment(ctx, cmdCtx)

	// imageRef, _ := cmdCtx.Config.GetString("image")

	// if imageRef == "" && cmdCtx.AppConfig != nil && cmdCtx.AppConfig.Build != nil && cmdCtx.AppConfig.Build.Image != "" {
	// 	imageRef = cmdCtx.AppConfig.Build.Image
	// }

	// var imageResolver imgsrc.ImageResolver

	// if imageRef != "" {
	// 	options := &imgsrc.RefOptions{
	// 		ImageRef:        imageRef,
	// 		AppNameOverride: cmdCtx.AppName,
	// 	}
	// 	options.ImageLabel, _ = cmdCtx.Config.GetString("image-label")

	// 	s, err := imgsrc.NewImageRefResolver(options)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	imageResolver = s
	// } else {
	// 	options := &imgsrc.SourceOptions{
	// 		WorkingDir:      cmdCtx.WorkingDir,
	// 		AppNameOverride: cmdCtx.AppName,
	// 		ExtraBuildArgs:  make(map[string]string),
	// 		AppConfig:       cmdCtx.AppConfig,
	// 	}

	// 	options.DockerfilePath, _ = cmdCtx.Config.GetString("dockerfile")
	// 	options.ImageLabel, _ = cmdCtx.Config.GetString("image-label")

	// 	extraArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
	// 	if err != nil {
	// 		return errors.Wrap(err, "invalid build-arg")
	// 	}
	// 	options.ExtraBuildArgs = extraArgs

	// 	resolver, err := imgsrc.NewSourceBuilder(options)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if resolver == nil {
	// 		return errors.New("project does not contain a Dockerfile or Buildpacks configuration")
	// 	}

	// 	imageResolver = resolver
	// }

	// if imageResolver == nil {
	// 	return errors.New("could not find an image to deploy")
	// }

	// fmt.Println("Deployment source:", imageResolver.Name())

	// if cmdCtx.Config.GetBool("remote-only") && cmdCtx.Config.GetBool("local-only") {
	// 	return errors.New("both --local-only and --remote-only are set - select only one")
	// }
	// dockerMode := imgsrc.Auto
	// if cmdCtx.Config.GetBool("remote-only") {
	// 	dockerMode = imgsrc.Remote
	// } else if cmdCtx.Config.GetBool("local-only") {
	// 	dockerMode = imgsrc.Local
	// }

	// if err := imageResolver.SetupDocker(ctx, cmdCtx.Client.API(), dockerMode); err != nil {
	// 	return err
	// }

	// img, err := imageResolver.Run(ctx, cmdCtx.IO)
	// if err != nil {
	// 	return err
	// }

	// cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image: %+v\n", img.Tag)
	// cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	// if cmdCtx.Config.GetBool("build-only") {
	// 	return nil
	// }

	// cmdCtx.Status("deploy", cmdctx.SBEGIN, "Creating Release")

	// input := api.DeployImageInput{
	// 	AppID: cmdCtx.AppName,
	// 	Image: img.Tag,
	// }
	// if val, _ := cmdCtx.Config.GetString("strategy"); val != "" {
	// 	input.Strategy = api.StringPointer(strings.ToUpper(val))
	// }
	// if cmdCtx.AppConfig != nil && len(cmdCtx.AppConfig.Definition) > 0 {
	// 	input.Definition = api.DefinitionPtr(cmdCtx.AppConfig.Definition)
	// }

	// release, err := cmdCtx.Client.API().DeployImage(input)
	// if err != nil {
	// 	return err
	// }

	// fmt.Printf("%+v\n", release)

	// // return err

	// // if needsCleaning {
	// // 	buildOp.CleanDeploymentTags(cmdCtx)
	// // }

	// cmdCtx.Statusf("deploy", cmdctx.SINFO, "Release v%d created\n", release.Version)
	// cmdCtx.Statusf("deploy", cmdctx.SINFO, "Deploying to %s.fly.dev\n", cmdCtx.AppName)

	// if strings.ToLower(release.DeploymentStrategy) == string(docker.ImmediateDeploymentStrategy) {
	// 	return nil
	// }

	// fmt.Println()

	// return watchDeployment(ctx, cmdCtx)

	// var strategy = docker.DefaultDeploymentStrategy
	// if val, _ := cmdCtx.Config.GetString("strategy"); val != "" {
	// 	strategy, err = docker.ParseDeploymentStrategy(val)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// var image *docker.Image

	// // imageRef, _ := cmdCtx.Config.GetString("image")

	// // if imageRef == "" &&
	// // 	cmdCtx.AppConfig != nil &&
	// // 	cmdCtx.AppConfig.Build != nil &&
	// // 	cmdCtx.AppConfig.Build.Image != "" {
	// // 	imageRef = cmdCtx.AppConfig.Build.Image
	// // }

	// buildOp, err := docker.NewBuildOperation(ctx, cmdCtx)
	// if err != nil {
	// 	return err
	// }

	// needsCleaning := false

	// if imageRef != "" {
	// 	// image specified, resolve it, tagging and pushing if docker+local
	// 	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Deploying image: %s\n", imageRef)

	// 	img, err := buildOp.ResolveImageLocally(ctx, cmdCtx, imageRef)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if img != nil {
	// 		image = img
	// 	} else {
	// 		image = &docker.Image{
	// 			Tag: imageRef,
	// 		}
	// 	}
	// } else {
	// 	needsCleaning = true
	// 	// no image specified, build one
	// 	buildArgs := map[string]string{}

	// 	if cmdCtx.AppConfig.Build != nil && cmdCtx.AppConfig.Build.Args != nil {
	// 		for k, v := range cmdCtx.AppConfig.Build.Args {
	// 			buildArgs[k] = v
	// 		}
	// 	}

	// 	for _, arg := range cmdCtx.Config.GetStringSlice("build-arg") {
	// 		parts := strings.Split(arg, "=")
	// 		if len(parts) != 2 {
	// 			return fmt.Errorf("Invalid build-arg '%s': must be in the format NAME=VALUE", arg)
	// 		}
	// 		buildArgs[parts[0]] = parts[1]
	// 	}

	// 	var dockerfilePath string

	// 	if dockerfile, _ := cmdCtx.Config.GetString("dockerfile"); dockerfile != "" {
	// 		dockerfilePath = dockerfile
	// 	}

	// 	if dockerfilePath == "" {
	// 		dockerfilePath = docker.ResolveDockerfile(cmdCtx.WorkingDir)
	// 	}

	// 	if dockerfilePath == "" && !cmdCtx.AppConfig.HasBuilder() && !cmdCtx.AppConfig.HasBuiltin() {
	// 		return docker.ErrNoDockerfile
	// 	}

	// 	if cmdCtx.AppConfig.HasBuilder() {
	// 		if dockerfilePath != "" {
	// 			terminal.Warn("Project contains both a Dockerfile and buildpacks, using buildpacks")
	// 		}
	// 	}

	// 	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Deploy source directory '%s'\n", cmdCtx.WorkingDir)

	// 	if cmdCtx.AppConfig.HasBuilder() {
	// 		cmdCtx.Status("deploy", cmdctx.SBEGIN, "Building with buildpacks")
	// 		img, err := buildOp.BuildWithPack(cmdCtx, buildArgs)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		image = img
	// 		cmdCtx.Status("deploy", cmdctx.SDONE, "Building with buildpacks done")
	// 	} else if cmdCtx.AppConfig.HasBuiltin() {
	// 		cmdCtx.Status("deploy", cmdctx.SBEGIN, "Building with Builtin")

	// 		img, err := buildOp.BuildWithDocker(cmdCtx, cmdCtx.WorkingDir, dockerfilePath, buildArgs)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		image = img
	// 		cmdCtx.Status("deploy", cmdctx.SDONE, "Building with Builtin done")
	// 	} else {
	// 		cmdCtx.Status("deploy", cmdctx.SBEGIN, "Building with Dockerfile")

	// 		img, err := buildOp.BuildWithDocker(cmdCtx, cmdCtx.WorkingDir, dockerfilePath, buildArgs)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		image = img
	// 		cmdCtx.Status("deploy", cmdctx.SDONE, "Building with Dockerfile done")
	// 	}
	// 	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image: %+v\n", image.Tag)
	// 	cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image size: %s\n", humanize.Bytes(uint64(image.Size)))

	// 	cmdCtx.Status("deploy", cmdctx.SBEGIN, "Pushing Image")
	// 	err := buildOp.PushImage(cmdCtx, *image)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	cmdCtx.Status("deploy", cmdctx.SDONE, "Done Pushing Image")

	// 	if cmdCtx.Config.GetBool("build-only") {
	// 		cmdCtx.Statusf("deploy", cmdctx.SINFO, "Image: %s\n", image.Tag)

	// 		return nil
	// 	}
	// }

	// if image == nil {
	// 	return errors.New("Could not find an image to deploy")
	// }

}

func watchDeployment(ctx context.Context, cmdCtx *cmdctx.CmdContext) error {
	if cmdCtx.Config.GetBool("detach") {
		return nil
	}

	cmdCtx.Status("deploy", cmdctx.STITLE, "Monitoring Deployment")
	cmdCtx.Status("deploy", cmdctx.SDETAIL, "You can detach the terminal anytime without stopping the deployment")

	interactive := cmdCtx.IO.IsInteractive()

	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(cmdCtx.Client.API(), cmdCtx.AppName)

	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			cmdCtx.StatusLn()
		}
		cmdCtx.Status("deploy", cmdctx.SINFO, presenters.FormatDeploymentSummary(d))

		return nil
	}

	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		if interactive && !cmdCtx.OutputJSON() {
			fmt.Fprint(cmdCtx.Out, aec.Up(1))
			fmt.Fprint(cmdCtx.Out, aec.EraseLine(aec.EraseModes.All))
			fmt.Fprintln(cmdCtx.Out, presenters.FormatDeploymentAllocSummary(d))
		} else {
			for _, alloc := range updatedAllocs {
				cmdCtx.Status("deploy", cmdctx.SINFO, presenters.FormatAllocSummary(alloc))
			}
		}

		return nil
	}

	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		cmdCtx.Statusf("deploy", cmdctx.SDETAIL, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if endmessage == "" && d.Status == "failed" {
			if strings.Contains(d.Description, "no stable job version to auto revert to") {
				endmessage = fmt.Sprintf("v%d %s - %s\n", d.Version, d.Status, d.Description)
			} else {
				endmessage = fmt.Sprintf("v%d %s - %s and deploying as v%d \n", d.Version, d.Status, d.Description, d.Version+1)
			}
		}

		if len(failedAllocs) > 0 {
			cmdCtx.Status("deploy", cmdctx.STITLE, "Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := cmdCtx.Client.API().GetAllocationStatus(cmdCtx.AppName, a.ID, 30)
					if err != nil {
						cmdCtx.Status("deploy", cmdctx.SERROR, "Error fetching alloc", a.ID, err)
						return
					}
					x <- alloc
				}()
			}

			go func() {
				wg.Wait()
				close(x)
			}()

			count := 0
			for alloc := range x {
				count++
				cmdCtx.StatusLn()
				cmdCtx.Statusf("deploy", cmdctx.SBEGIN, "Failure #%d\n", count)
				cmdCtx.StatusLn()

				err := cmdCtx.Frender(
					cmdctx.PresenterOption{
						Title: "Instance",
						Presentable: &presenters.Allocations{
							Allocations: []*api.AllocationStatus{alloc},
						},
						Vertical: true,
					},
					cmdctx.PresenterOption{
						Title: "Recent Events",
						Presentable: &presenters.AllocationEvents{
							Events: alloc.Events,
						},
					},
				)
				if err != nil {
					return err
				}

				cmdCtx.Status("deploy", cmdctx.STITLE, "Recent Logs")
				logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
				logPresenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), alloc.RecentLogs)
			}

		}

		return nil
	}

	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		cmdCtx.Statusf("deploy", cmdctx.SDONE, "v%d deployed successfully\n", d.Version)
		return nil
	}

	monitor.Start(ctx)

	if err := monitor.Error(); err != nil {
		return err
	}

	if endmessage != "" {
		cmdCtx.Status("deploy", cmdctx.SERROR, endmessage)
	}

	if !monitor.Success() {
		cmdCtx.Status("deploy", cmdctx.SINFO, "Troubleshooting guide at https://fly.io/docs/getting-started/troubleshooting/")
		return ErrAbort
	}

	return nil
}
