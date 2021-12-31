package deploy

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

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/logs"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/format"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/logger"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Deploy Fly applications from source or an image using a local or remote builder.
	`
		short = "Deploy Fly applications"
	)

	cmd = command.New("deploy [WORKING_DIRECTORY]", short, long, run,
		command.RequireSession,
		swapWorkingDirectoryIfRequired,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(),
		flag.LocalOnly(),
		flag.BuildOnly(),
		flag.Detach(),
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
			Description: `Image label to use when tagging and pushing to the fly registry. Defaults to "deployment-{timestamp}".`,
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

	return
}

func swapWorkingDirectoryIfRequired(ctx context.Context) (context.Context, error) {
	wd := flag.FirstArg(ctx)
	if wd == "" {
		return ctx, nil
	}

	if !filepath.IsAbs(wd) {
		p, err := filepath.Abs(wd)
		if err != nil {
			return nil, fmt.Errorf("failed converting %s to an absolute path: %w", wd, err)
		}
		wd = p
	}

	if err := os.Chdir(wd); err != nil {
		return nil, fmt.Errorf("failed changing working directory: %w", err)
	}

	return state.WithWorkingDirectory(ctx, wd), nil
}

func run(ctx context.Context) error {
	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		return err
	}

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImage(ctx, appConfig)
	if err != nil {
		return fmt.Errorf("failed to fetch an image or build from source: %w", err)
	}

	if flag.GetBuildOnly(ctx) {
		return nil
	}

	release, releaseCommand, err := createRelease(ctx, img)
	if err != nil {
		return err
	}

	if flag.GetDetach(ctx) {
		return nil
	}

	// TODO: This is a single message that doesn't belong to any block output, so we should have helpers to allow that
	tb := render.NewTextBlock(ctx)
	tb.Done("You can detach the terminal anytime without stopping the deployment")

	// Run the pre-deployment release command if it's set
	if releaseCommand != nil {
		// TODO: don't use text block here
		tb := render.NewTextBlock(ctx, fmt.Sprintf("Release command detected: %s\n", releaseCommand.Command))
		tb.Done("This release will not be available until the release command succeeds.")

		if err := watchReleaseCommand(ctx, releaseCommand.ID); err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		logger := logger.FromContext(ctx)
		logger.Debug("immediate deployment strategy, nothing to monitor")

		return nil
	}

	err = watchDeployment(ctx)

	return err
}

// determineAppConfig fetching the app config from a local file, or in its absence, from the API
func determineAppConfig(ctx context.Context) (cfg *app.Config, err error) {
	tb := render.NewTextBlock(ctx, "Verifying app config")

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

	tb.Done("Verified app config")

	return
}

func determineImage(ctx context.Context, appConfig *app.Config) (img *imgsrc.DeploymentImage, err error) {
	tb := render.NewTextBlock(ctx, "Building image")
	daemonType := imgsrc.NewDockerDaemonType(!flag.GetRemoteOnly(ctx), !flag.GetLocalOnly(ctx))
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	var imageRef string
	if imageRef, err = fetchImageRef(ctx, app.ConfigFromContext(ctx)); err != nil {
		return
	}

	// we're using a pre-built Docker image
	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    app.NameFromContext(ctx),
			WorkingDir: state.WorkingDirectory(ctx),
			Publish:    !flag.GetBuildOnly(ctx),
			ImageRef:   imageRef,
			ImageLabel: flag.GetString(ctx, "image-label"),
		}

		img, err = resolver.ResolveReference(ctx, io, opts)

		return
	}

	build := appConfig.Build
	if build == nil {
		build = new(app.Build)
	}

	var buildArgs map[string]string
	if buildArgs, err = mergeBuildArgs(ctx, build.Args); err != nil {
		return
	}

	// We're building from source
	opts := imgsrc.ImageOptions{
		AppName:         app.NameFromContext(ctx),
		WorkingDir:      state.WorkingDirectory(ctx),
		Publish:         !flag.GetBuildOnly(ctx),
		ImageLabel:      flag.GetString(ctx, "image-label"),
		NoCache:         flag.GetBool(ctx, "no-cache"),
		BuildArgs:       buildArgs,
		BuiltIn:         build.Builtin,
		BuiltInSettings: build.Settings,
		Builder:         build.Builder,
		Buildpacks:      build.Buildpacks,
	}

	if opts.DockerfilePath, err = resolveDockerfilePath(ctx, appConfig); err != nil {
		return
	}

	if target := appConfig.DockerBuildTarget(); target != "" {
		opts.Target = target
	} else if target := flag.GetString(ctx, "build-target"); target != "" {
		opts.Target = target
	}

	// finally, build the image
	if img, err = resolver.BuildImage(ctx, io, opts); err == nil && img == nil {
		err = errors.New("no image specified")
	}

	if err == nil {
		tb.Printf("image: %s\n", img.Tag)
		tb.Printf("image size: %s\n", humanize.Bytes(uint64(img.Size)))
	}

	return
}

// resolveDockerfilePath returns the absolute path to the Dockerfile
// if one was specified in the app config or a command line argument
func resolveDockerfilePath(ctx context.Context, appConfig *app.Config) (path string, err error) {
	defer func() {
		if err == nil && path != "" {
			path, err = filepath.Abs(path)
		}
	}()

	if path = appConfig.Dockerfile(); path != "" {
		path = filepath.Join(filepath.Dir(appConfig.Path), path)
	} else {
		path = flag.GetString(ctx, "dockerfile")
	}

	return
}

func mergeBuildArgs(ctx context.Context, args map[string]string) (map[string]string, error) {
	if args == nil {
		args = make(map[string]string)
	}

	// set additional Docker build args from the command line, overriding similar ones from the config
	cliBuildArgs, err := cmdutil.ParseKVStringsToMap(flag.GetStringSlice(ctx, "build-arg"))
	if err != nil {
		return nil, fmt.Errorf("invalid build args: %w", err)
	}

	for k, v := range cliBuildArgs {
		args[k] = v
	}

	return args, nil
}

func fetchImageRef(ctx context.Context, cfg *app.Config) (ref string, err error) {
	if ref = flag.GetString(ctx, "image"); ref != "" {
		return
	}

	if cfg != nil && cfg.Build != nil {
		if ref = cfg.Build.Image; ref != "" {
			return
		}
	}

	return ref, nil
}

func createRelease(ctx context.Context, img *imgsrc.DeploymentImage) (*api.Release, *api.ReleaseCommand, error) {
	tb := render.NewTextBlock(ctx, "Creating release")
	appConfig := app.ConfigFromContext(ctx)

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
	client := client.FromContext(ctx).API()

	release, releaseCommand, err := client.DeployImage(ctx, input)
	if err == nil {
		tb.Donef("release v%d created\n", release.Version)
	}

	return release, releaseCommand, err
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
	tb := render.NewTextBlock(ctx, "Monitoring deployment")

	io := iostreams.FromContext(ctx)
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(client, appName)

	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			tb.Println()
		}
		tb.Println(format.DeploymentSummary(d))

		return nil
	}

	// TODO check we aren't asking for JSON
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		if io.IsInteractive() {
			tb.Overwrite()

			tb.Println(format.DeploymentAllocSummary(d))
		} else {
			for _, alloc := range updatedAllocs {
				tb.Println(format.AllocSummary(alloc))
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
			tb.Println("Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := client.GetAllocationStatus(ctx, appName, a.ID, 30)
					if err != nil {
						//cmdCtx.Status("deploy", cmdctx.SERROR, "Error fetching alloc", a.ID, err)
						tb.Printf("failed fetching alloc %s: %s", a.ID, err)

						return
					}
					x <- alloc
				}()
			}

			go func() {
				wg.Wait()
				close(x)
			}()

			var count int
			for alloc := range x {
				count++

				tb.Println()
				tb.Printf("Failure #%d\n", count)
				tb.Println()

				as, err := renderAllocationStatus(nil, alloc)
				if err != nil {
					return fmt.Errorf("failed rendering alloc status: %w", err)
				}
				tb.Println(as)

				re, err := renderRecentEvents(alloc.Events)
				if err != nil {
					return fmt.Errorf("failed rendering recent events: %w", err)
				}
				tb.Println(re)

				renderLogs(ctx, alloc)
			}
		}

		return nil
	}

	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		tb.Donef("v%d deployed successfully\n", d.Version)

		return nil
	}

	monitor.Start(ctx)

	if err := monitor.Error(); err != nil {
		return err
	}

	if endmessage != "" {
		tb.Done(endmessage)
	}

	if !monitor.Success() {
		tb.Done("Troubleshooting guide at https://fly.io/docs/getting-started/troubleshooting/")
		return flyerr.ErrAbort
	}

	return nil
}

func renderAllocationStatus(br []api.Region, s ...*api.AllocationStatus) (string, error) {
	var sb strings.Builder
	err := render.AllocationStatuses(&sb, "Instance", br, s...)

	return sb.String(), err
}

func renderRecentEvents(events []api.AllocationEvent) (string, error) {
	var sb strings.Builder
	err := render.AllocationEvents(&sb, "Recent Events", events...)

	return sb.String(), err
}

func renderLogs(ctx context.Context, alloc *api.AllocationStatus) {
	out := iostreams.FromContext(ctx).Out
	cfg := config.FromContext(ctx).JSONOutput

	logPresenter := presenters.LogPresenter{
		HideAllocID:    true,
		HideRegion:     true,
		RemoveNewlines: true,
	}

	for _, e := range alloc.RecentLogs {
		entry := logs.LogEntry{
			Instance:  e.Instance,
			Level:     e.Level,
			Message:   e.Message,
			Region:    e.Region,
			Timestamp: e.Timestamp,
			Meta:      e.Meta,
		}

		logPresenter.FPrint(out, cfg, entry)
	}
}
