package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/logs"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/terminal"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Deploy Fly applications from source or an image using a local or remote builder.
	`
		short = "Deploy Fly applications"
	)

	cmd = command.New("deploy", short, long, run,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(),
		flag.String{
			Name:        "strategy",
			Description: "The strategy for replacing running instances. Options are canary, rolling, bluegreen, or immediate. Default is canary, or rolling when max-per-region is set.",
		},
		flag.String{
			Name:        "dockerfile",
			Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
		},
		flag.StringSlice{
			Name:        "env",
			Shorthand:   "e",
			Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.String{
			Name:        "image-label",
			Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
		},
		flag.StringSlice{
			Name:        "build-arg",
			Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.String{
			Name:        "build-target",
			Description: "Set the target build stage to build if the Dockerfile has more than one stage",
		},
		flag.Bool{
			Name:        "no-cache",
			Description: "Do not use the build cache when building the image",
		},
	)

	// TODO: see why we need working directory
	cmd.Args = cobra.MaximumNArgs(1)

	return
}

func run(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	// Fetch and validate the app config
	cmdfmt.Begin(ctx, "validating app configuration ...")

	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		return err
	}

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImage(ctx, appConfig)

	if err != nil {
		return fmt.Errorf("failed to fetch an image or build from source: %w", err)
	}

	cmdfmt.Printf(ctx, "image: %s\n", img.Tag)
	cmdfmt.Printf(ctx, "image size: %s\n", humanize.Bytes(uint64(img.Size)))

	if flag.GetBool(ctx, "build-only") {
		return nil
	}

	cmdfmt.Begin(ctx, "creating release ...")

	input := api.DeployImageInput{
		AppID: app.NameFromContext(ctx),
		Image: img.Tag,
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ToUpper(val))
	}

	if appConfig != nil && len(appConfig.Definition) > 0 {
		input.Definition = api.DefinitionPtr(appConfig.Definition)
	}

	// Start deployment of the determined image
	release, releaseCommand, err := client.DeployImage(ctx, input)

	if err != nil {
		return err
	}

	cmdfmt.Donef(ctx, "release v%d created\n", release.Version)

	if releaseCommand != nil {
		cmdfmt.Println(ctx, "release command detected: this new release will not be available until the command succeeds")
	}

	if flag.GetBool(ctx, "detach") {
		return nil
	}

	cmdfmt.Println(ctx, "You can detach the terminal anytime without stopping the deployment")
	// cmdCtx.Status("deploy", cmdctx.SDETAIL, "You can detach the terminal anytime without stopping the deployment")

	// Run the pre-deployment release command if it's set
	if releaseCommand != nil {
		cmdfmt.Begin(ctx, "Release command: %s\n", releaseCommand.Command)

		if err := watchReleaseCommand(ctx, releaseCommand.ID); err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		terminal.Debug("immediate deployment strategy, nothing to monitor")

		return nil
	}

	err = watchDeployment(ctx)

	return err
}

// determineAppConfig fetching the app config from a local file, or in its absence, from the API
func determineAppConfig(ctx context.Context) (cfg *app.Config, err error) {
	client := client.FromContext(ctx).API()

	if cfg = app.ConfigFromContext(ctx); cfg == nil {
		var apiConfig *api.AppConfig
		if apiConfig, err = client.GetConfig(ctx, app.NameFromContext(ctx)); err != nil {
			err = fmt.Errorf("failed fetching existing app config: %w", err)

			return
		}

		cfg = &app.Config{
			Definition: apiConfig.Definition,
		}
	}

	if env := flag.GetStringSlice(ctx, "env"); len(env) > 0 {
		var parsedEnv map[string]string
		if parsedEnv, err = cmdutil.ParseKVStringsToMap(env); err != nil {
			err = fmt.Errorf("failed parsing environment: %w", err)

			return
		}

		cfg.SetEnvVariables(parsedEnv)
	}

	return
}

// determineImage fetches an optional imagef
func determineImage(ctx context.Context, appConfig *app.Config) (img *imgsrc.DeploymentImage, err error) {
	daemonType := imgsrc.NewDockerDaemonType(!flag.GetBool(ctx, "remote-only"), !flag.GetBool(ctx, "local-only"))

	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	var imageRef string
	if imageRef, err = fetchImageRef(ctx, app.ConfigFromContext(ctx)); err != nil {
		return
	}

	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    app.NameFromContext(ctx),
			WorkingDir: state.WorkingDirectory(ctx),
			Publish:    !flag.GetBool(ctx, "build-only"),
			ImageRef:   imageRef,
			ImageLabel: flag.GetString(ctx, "image-label"),
		}

		img, err = resolver.ResolveReference(ctx, io, opts)

		return
	}

	opts := imgsrc.ImageOptions{
		AppName:    app.NameFromContext(ctx),
		WorkingDir: state.WorkingDirectory(ctx),
		Publish:    !flag.GetBool(ctx, "build-only"),
		ImageLabel: flag.GetString(ctx, "image-label"),
		NoCache:    flag.GetBool(ctx, "no-cache"),
	}

	// A Dockerfile was specified in the config, so set the path relative to the directory containing the config file
	// Otherwise, use the absolute path to the Dockerfile specified on the command line
	if path := appConfig.Dockerfile(); path != "" {
		opts.DockerfilePath = filepath.Join(filepath.Dir(appConfig.Path), path)
	} else if path := flag.GetString(ctx, "dockerfile"); path != "" {
		if path, err = filepath.Abs(path); err != nil {
			return
		}
		opts.DockerfilePath = path
	}

	if target := appConfig.DockerBuildTarget(); target != "" {
		opts.Target = target
	} else if target := flag.GetString(ctx, "build-target"); target != "" {
		opts.Target = target
	}

	// Set Docker build args
	var buildArgs map[string]string
	if buildArgs, err = cmdutil.ParseKVStringsToMap(flag.GetStringSlice(ctx, "build-arg")); err != nil {
		err = fmt.Errorf("invalid build args: %w", err)

		return
	}

	opts.ExtraBuildArgs = buildArgs

	// Finally, build the image
	if img, err = resolver.BuildImage(ctx, io, opts); err == nil && img == nil {
		err = errors.New("no image specified")
	}

	return
}

func fetchImageRef(ctx context.Context, cfg *app.Config) (ref string, err error) {
	if ref = flag.GetString(ctx, "image"); ref != "" {
		return
	}

	if cfg.Build != nil {
		if ref = cfg.Build.Image; ref != "" {
			return
		}
	}

	return ref, nil
}

func watchReleaseCommand(ctx context.Context, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	interactive := io.IsInteractive()
	appName := app.NameFromContext(ctx)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Running release task..."

	if interactive {
		s.Start()
		defer s.Stop()
	}

	rcUpdates := make(chan api.ReleaseCommand)

	var once sync.Once

	startLogs := func(ctx context.Context, vmid string) {
		once.Do(func() {
			g.Go(func() error {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				opts := &logs.LogOptions{MaxBackoff: 1 * time.Second, AppName: appName, VMID: vmid}
				ls, err := logs.NewPollingStream(ctx, client, opts)
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
			rc, err := client.GetReleaseCommand(ctx, id)
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
			if interactive {
				s.Prefix = fmt.Sprintf("Running release task (%s)...", rc.Status)
			}

			if rc.InstanceID != nil {
				startLogs(logsCtx, *rc.InstanceID)
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

func watchDeployment(ctx context.Context) error {
	// cmdCtx.Status("deploy", cmdctx.STITLE, "Monitoring Deployment")

	io := iostreams.FromContext(ctx)
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx)

	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(appName)

	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			cmdfmt.Separator(ctx)
		}
		// cmdCtx.Status("deploy", cmdctx.SINFO, presenters.FormatDeploymentSummary(d))
		cmdfmt.Println(ctx, presenters.FormatDeploymentSummary(d))
		return nil
	}

	// TODO check we aren't asking for JSON
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		if io.IsInteractive() {
			cmdfmt.Overwrite(ctx)
			cmdfmt.Println(ctx, presenters.FormatDeploymentAllocSummary(d))
		} else {
			for _, alloc := range updatedAllocs {
				//	cmdCtx.Status("deploy", cmdctx.SINFO, presenters.FormatAllocSummary(alloc))
				cmdfmt.Println(ctx, presenters.FormatAllocSummary(alloc))
			}
		}

		return nil
	}

	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		// cmdCtx.Statusf("deploy", cmdctx.SDETAIL, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if endmessage == "" && d.Status == "failed" {
			if strings.Contains(d.Description, "no stable release to revert to") {
				endmessage = fmt.Sprintf("v%d %s - %s\n", d.Version, d.Status, d.Description)
			} else {
				endmessage = fmt.Sprintf("v%d %s - %s and deploying as v%d \n", d.Version, d.Status, d.Description, d.Version+1)
			}
		}

		if len(failedAllocs) > 0 {

			//cmdCtx.Status("deploy", cmdctx.STITLE, "Failed Instances")
			cmdfmt.Println(ctx, "Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := client.API().GetAllocationStatus(ctx, appName, a.ID, 30)
					if err != nil {
						//cmdCtx.Status("deploy", cmdctx.SERROR, "Error fetching alloc", a.ID, err)
						cmdfmt.Println(ctx, "Error fetching alloc", a.ID, err)

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
				cmdfmt.Separator(ctx)
				//cmdCtx.Statusf("deploy", cmdctx.SBEGIN, "Failure #%d\n", count)
				cmdfmt.Println(ctx, "Failure #%d\n", count)
				cmdfmt.Separator(ctx)

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
