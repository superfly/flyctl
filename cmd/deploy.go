package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/logs"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
)

func runDeploy(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	cmdCtx.Status("deploy", cmdctx.STITLE, "Deploying", cmdCtx.AppName)

	cmdfmt.PrintBegin(cmdCtx.Out, "Validating app configuration")

	if cmdCtx.AppConfig == nil {
		cfg, err := cmdCtx.Client.API().GetConfig(ctx, cmdCtx.AppName)
		if err != nil {
			return fmt.Errorf("unable to fetch existing configuration file: %s", err)
		}

		cmdCtx.AppConfig.Definition = cfg.Definition
	}

	if extraEnv := cmdCtx.Config.GetStringSlice("env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("env"))
		if err != nil {
			return errors.Wrap(err, "invalid env")
		}
		cmdCtx.AppConfig.SetEnvVariables(parsedEnv)
	}

	parsedCfg, err := cmdCtx.Client.API().ParseConfig(ctx, cmdCtx.AppName, cmdCtx.AppConfig.Definition)
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
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageRef:   imageRef,
			ImageLabel: cmdCtx.Config.GetString("image-label"),
		}

		img, err = resolver.ResolveReference(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
	} else {
		buildArgs := make(map[string]string)

		// Add placeholder build-args for secrets
		secrets, err := cmdCtx.Client.API().GetAppSecrets(ctx, cmdCtx.AppName)
		if err != nil {
			return err
		}

		for _, secret := range secrets {
			buildArgs[secret.Name] = "<placeholder-secret>"
		}

		// Add placeholder build-args for envs
		var env map[string]interface{}

		if rawEnv, ok := cmdCtx.AppConfig.Definition["env"]; ok {
			if castEnv, ok := rawEnv.(map[string]interface{}); ok {
				env = castEnv
			}
		}

		for key := range env {
			buildArgs[key] = "<placeholder-env>"
		}

		opts := imgsrc.ImageOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageLabel: cmdCtx.Config.GetString("image-label"),
			NoCache:    cmdCtx.Config.GetBool("no-cache"),
		}

		if cmdCtx.AppConfig.Build != nil {
			opts.BuiltIn = cmdCtx.AppConfig.Build.Builtin
			opts.BuiltInSettings = cmdCtx.AppConfig.Build.Settings
			opts.Builder = cmdCtx.AppConfig.Build.Builder
			opts.Buildpacks = cmdCtx.AppConfig.Build.Buildpacks

			if cmdCtx.AppConfig.Build.Args != nil {
				for k, v := range cmdCtx.AppConfig.Build.Args {
					buildArgs[k] = v
				}
			}
		}

		cliBuildArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
		if err != nil {
			return errors.Wrap(err, "invalid build-arg")
		}

		for k, v := range cliBuildArgs {
			buildArgs[k] = v
		}

		opts.BuildArgs = buildArgs

		if dockerfilePath := cmdCtx.Config.GetString("dockerfile"); dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return err
			}
			opts.DockerfilePath = dockerfilePath
		} else if dockerfilePath := cmdCtx.AppConfig.Dockerfile(); dockerfilePath != "" {
			opts.DockerfilePath = filepath.Join(filepath.Dir(cmdCtx.ConfigFile), dockerfilePath)
		}

		if dockerBuildTarget := cmdCtx.Config.GetString("build-target"); dockerBuildTarget != "" {
			opts.Target = dockerBuildTarget
		} else if dockerBuildTarget := cmdCtx.AppConfig.DockerBuildTarget(); dockerBuildTarget != "" {
			opts.Target = dockerBuildTarget
		}

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

	release, releaseCommand, err := cmdCtx.Client.API().DeployImage(ctx, input)
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

		release, err = cmdCtx.Client.API().GetAppRelease(ctx, cmdCtx.AppName, release.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		terminal.Debug("immediate deployment strategy, nothing to monitor")
		return nil
	}

	return watchDeployment(ctx, cmdCtx, release.EvaluationID)
}

func watchReleaseCommand(ctx context.Context, cc *cmdctx.CmdContext, apiClient *api.Client, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	interactive := cc.IO.IsInteractive()

	s := spinner.Run(cc.IO, "Running release task...")
	defer s.Stop()

	rcUpdates := make(chan api.ReleaseCommand)

	var once sync.Once

	startLogs := func(ctx context.Context, vmid string) {
		once.Do(func() {
			g.Go(func() error {
				childCtx, cancel := context.WithCancel(ctx)
				defer cancel()

				opts := &logs.LogOptions{MaxBackoff: 1 * time.Second, AppName: cc.AppName, VMID: vmid}
				ls, err := logs.NewPollingStream(apiClient, opts)
				if err != nil {
					return err
				}

				for entry := range ls.Stream(childCtx, opts) {
					msg := s.Stop()

					fmt.Println("\t", entry.Message)

					// watch for the shutdown message
					if entry.Message == "Starting clean up." {
						cancel()
					}

					s.StartWithMessage(msg)
				}

				if err = ls.Err(); (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && ctx.Err() == nil {
					err = nil
				}
				return err
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
		// The logs goroutine will stop itself when it sees a shutdown log message.
		// If the message never comes (delayed logs, etc) the deploy will hang.
		// This timeout makes sure they always stop a few seconds after the release task is done.
		logsCtx, logsCancel := context.WithCancel(ctx)
		defer time.AfterFunc(3*time.Second, logsCancel)

		for rc := range rcUpdates {
			msg := fmt.Sprintf("Running release task (%s)...", rc.Status)
			s.Set(msg)

			if rc.InstanceID != nil {
				startLogs(logsCtx, *rc.InstanceID)
			}

			if !rc.InProgress && rc.Failed {
				if rc.Succeeded && interactive {
					s.StopWithMessage("Running release task... Done.")
				} else if rc.Failed {
					return errors.New("Release command failed, deployment aborted")
				}
			}
		}

		return nil
	})

	return g.Wait()
}

func watchDeployment(ctx context.Context, cmdCtx *cmdctx.CmdContext, evaluationID string) error {
	cmdCtx.Status("deploy", cmdctx.STITLE, "Monitoring Deployment")

	interactive := cmdCtx.IO.IsInteractive()

	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(cmdCtx.Client.API(), cmdCtx.AppName, evaluationID)

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
					alloc, err := cmdCtx.Client.API().GetAllocationStatus(ctx, cmdCtx.AppName, a.ID, 30)
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

				for _, e := range alloc.RecentLogs {
					entry := logs.LogEntry{
						Instance:  e.Instance,
						Level:     e.Level,
						Message:   e.Message,
						Region:    e.Region,
						Timestamp: e.Timestamp,
						Meta:      e.Meta,
					}
					logPresenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), entry)
				}
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
