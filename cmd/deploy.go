package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
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
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
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
		Name: "build-only",
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
		Description: "The strategy for replacing running instances. Options are canary, rolling, bluegreen, or immediate. Default is canary, or rolling when max-per-region is set.",
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
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "build-target",
		Description: "Set the target build stage to build if the Dockerfile has more than one stage",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "no-cache",
		Description: "Do not use the cache when building the image",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	cmdCtx.Status("deploy", cmdctx.STITLE, "Deploying", cmdCtx.AppName)

	cmdfmt.PrintBegin(cmdCtx.Out, "Validating app configuration")

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
	cmdfmt.PrintDone(cmdCtx.Out, "Validating app configuration done")

	if parsedCfg.Valid && len(parsedCfg.Services) > 0 {
		cmdfmt.PrintServicesList(cmdCtx.IO, parsedCfg.Services)
	}

	daemonType := imgsrc.NewDockerDaemonType(!cmdCtx.Config.GetBool("remote-only"), !cmdCtx.Config.GetBool("local-only"))
	resolver := imgsrc.NewResolver(daemonType, cmdCtx.Client.API(), cmdCtx.AppName, cmdCtx.IO)

	var img *imgsrc.DeploymentImage

	var imageRef string
	if ref := cmdCtx.Config.GetString("image"); ref != "" {
		imageRef = ref
	} else if ref := cmdCtx.AppConfig.Image(); ref != "" {
		imageRef = ref
	}

	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			AppConfig:  cmdCtx.AppConfig,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageRef:   imageRef,
			ImageLabel: cmdCtx.Config.GetString("image-label"),
		}

		img, err = resolver.ResolveReference(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
	} else {
		opts := imgsrc.ImageOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			AppConfig:  cmdCtx.AppConfig,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageLabel: cmdCtx.Config.GetString("image-label"),
			Target:     cmdCtx.Config.GetString("build-target"),
			NoCache:    cmdCtx.Config.GetBool("no-cache"),
		}
		if dockerfilePath := cmdCtx.Config.GetString("dockerfile"); dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return err
			}
			opts.DockerfilePath = dockerfilePath
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
		if err != nil {
			return errors.Wrap(err, "invalid build-arg")
		}
		opts.ExtraBuildArgs = extraArgs

		img, err = resolver.BuildImage(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
		if img == nil {
			return errors.New("could not find an image to deploy")
		}
	}

	if img == nil {
		return errors.New("could not find an image to deploy")
	}

	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image: %s\n", img.Tag)
	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	if cmdCtx.Config.GetBool("build-only") {
		return nil
	}

	cmdfmt.PrintBegin(cmdCtx.Out, "Creating release")

	input := api.DeployImageInput{
		AppID: cmdCtx.AppName,
		Image: img.Tag,
	}
	if val := cmdCtx.Config.GetString("strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ToUpper(val))
	}
	if cmdCtx.AppConfig != nil && len(cmdCtx.AppConfig.Definition) > 0 {
		input.Definition = api.DefinitionPtr(cmdCtx.AppConfig.Definition)
	}

	release, releaseCommand, err := cmdCtx.Client.API().DeployImage(input)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintf(cmdCtx.Out, "Release command detected: this new release will not be available until the command succeeds.\n")
	}

	if cmdCtx.Config.GetBool("detach") {
		return nil
	}

	fmt.Println()
	cmdCtx.Status("deploy", cmdctx.SDETAIL, "You can detach the terminal anytime without stopping the deployment")

	if releaseCommand != nil {
		cmdfmt.PrintBegin(cmdCtx.Out, "Release command")
		fmt.Printf("Command: %s\n", releaseCommand.Command)

		err = watchReleaseCommand(ctx, cmdCtx, cmdCtx.Client.API(), releaseCommand.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		terminal.Debug("immediate deployment strategy, nothing to monitor")
		return nil
	}

	return watchDeployment(ctx, cmdCtx)
}

func watchReleaseCommand(ctx context.Context, cc *cmdctx.CmdContext, apiClient *api.Client, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	interactive := cc.IO.IsInteractive()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Running release task..."

	if interactive {
		s.Start()
		defer s.Stop()
	}

	rcUpdates := make(chan api.ReleaseCommand)

	var once sync.Once

	startLogs := func(vmid string) {
		once.Do(func() {
			g.Go(func() error {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				opts := &logs.LogOptions{MaxBackoff: 1 * time.Second, AppName: cc.AppName, VMID: vmid}
				ls, err := logs.NewPollingStream(apiClient, opts)
				if err != nil {
					return err
				}

				for entry := range ls.Stream(ctx, opts) {

					func() {
						if interactive {
							s.Stop()
							defer s.Start()
						}

						fmt.Println("\t", entry.Message)

						// watch for the shutdown message
						if entry.Message == "Starting clean up." {
							cancel()
						}

					}()
				}
				return ls.Err()
			})
		})
	}

	g.Go(func() error {
		var lastValue *api.ReleaseCommand
		var errorCount int
		defer close(rcUpdates)

		for {
			rc, err := apiClient.GetReleaseCommand(ctx, id)
			if err != nil {
				errorCount += 1
				if errorCount < 3 {
					continue
				}
				return err
			}

			if !reflect.DeepEqual(lastValue, rc) {
				lastValue = rc
				rcUpdates <- *rc
			}

			if !rc.InProgress {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		return nil
	})

	g.Go(func() error {
		for rc := range rcUpdates {
			if interactive {
				s.Prefix = fmt.Sprintf("Running release task (%s)...", rc.Status)
			}

			if rc.InstanceID != nil {
				startLogs(*rc.InstanceID)
			}

			if !rc.InProgress && rc.Failed {
				if rc.Succeeded && interactive {
					s.FinalMSG = "Running release task...Done\n"
				} else if rc.Failed {
					return errors.New("Release command failed, deployment aborted")
				}
			}
		}

		return nil
	})

	return g.Wait()
}

func watchDeployment(ctx context.Context, cmdCtx *cmdctx.CmdContext) error {
	cmdCtx.Status("deploy", cmdctx.STITLE, "Monitoring Deployment")

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
			if strings.Contains(d.Description, "no stable release to revert to") {
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
				// logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
				// logPresenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), alloc.RecentLogs)
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
		return flyerr.ErrAbort
	}

	return nil
}
