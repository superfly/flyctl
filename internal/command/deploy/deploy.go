package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/watch"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Deploy Fly applications from source or an image using a local or remote builder.

		To disable colorized output and show full Docker build output, set the environment variable NO_COLOR=1.
	`
		short = "Deploy Fly applications"
	)

	cmd = command.New("deploy [WORKING_DIRECTORY]", short, long, run,
		command.RequireSession,
		command.ChangeWorkingDirectoryToFirstArgIfPresent,
		command.RequireAppName,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(false),
		flag.LocalOnly(),
		flag.Nixpacks(),
		flag.BuildOnly(),
		flag.Push(),
		flag.Detach(),
		flag.Strategy(),
		flag.Dockerfile(),
		flag.StringSlice{
			Name:        "env",
			Shorthand:   "e",
			Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
		},
		flag.ImageLabel(),
		flag.BuildArg(),
		flag.BuildSecret(),
		flag.BuildTarget(),
		flag.NoCache(),
	)

	return
}

func run(ctx context.Context) error {
	appConfig, err := determineAppConfig(ctx)
	if err != nil {
		return err
	}

	return DeployWithConfig(ctx, appConfig)
}

func DeployWithConfig(ctx context.Context, appConfig *app.Config) (err error) {
	apiClient := client.FromContext(ctx).API()

	// Assign an empty map if nil so later assignments won't fail
	if appConfig.Env == nil {
		appConfig.Env = map[string]string{}
	}

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImage(ctx, appConfig)
	if err != nil {
		return fmt.Errorf("failed to fetch an image or build from source: %w", err)
	}

	if flag.GetBuildOnly(ctx) {
		return nil
	}

	var release *api.Release
	var releaseCommand *api.ReleaseCommand

	if appConfig.PrimaryRegion != "" && appConfig.Env["PRIMARY_REGION"] == "" {
		appConfig.Env["PRIMARY_REGION"] = appConfig.PrimaryRegion
	}

	if appConfig.ForMachines() {
		return createMachinesRelease(ctx, appConfig, img, flag.GetString(ctx, "strategy"))
	}

	release, releaseCommand, err = createRelease(ctx, appConfig, img)
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

		if err := watch.ReleaseCommand(ctx, releaseCommand.ID); err != nil {
			return err
		}

		release, err = apiClient.GetAppRelease(ctx, app.NameFromContext(ctx), release.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		logger := logger.FromContext(ctx)
		logger.Debug("immediate deployment strategy, nothing to monitor")

		return nil
	}

	err = watch.Deployment(ctx, app.NameFromContext(ctx), release.EvaluationID)

	return err
}

// determineAppConfig fetches the app config from a local file, or in its absence, from the API
func determineAppConfig(ctx context.Context) (cfg *app.Config, err error) {
	tb := render.NewTextBlock(ctx, "Verifying app config")
	client := client.FromContext(ctx).API()

	if cfg = app.ConfigFromContext(ctx); cfg == nil {
		logger := logger.FromContext(ctx)
		logger.Debug("no local app config detected; fetching from backend ...")

		var apiConfig *api.AppConfig
		if apiConfig, err = client.GetConfig(ctx, app.NameFromContext(ctx)); err != nil {
			err = fmt.Errorf("failed fetching existing app config: %w", err)
			return
		}

		basicApp, err := client.GetAppBasic(ctx, app.NameFromContext(ctx))
		if err != nil {
			return nil, err
		}

		cfg = &app.Config{
			Definition: apiConfig.Definition,
		}

		cfg.AppName = basicApp.Name
		cfg.SetPlatformVersion(basicApp.PlatformVersion)
	}

	if env := flag.GetStringSlice(ctx, "env"); len(env) > 0 {
		var parsedEnv map[string]string
		if parsedEnv, err = cmdutil.ParseKVStringsToMap(env); err != nil {
			err = fmt.Errorf("failed parsing environment: %w", err)

			return
		}
		cfg.SetEnvVariables(parsedEnv)
	}

	if regionCode := flag.GetString(ctx, flag.RegionName); regionCode != "" {
		cfg.PrimaryRegion = regionCode
	}

	tb.Done("Verified app config")
	return
}

// determineImage picks the deployment strategy, builds the image and returns a
// DeploymentImage struct
func determineImage(ctx context.Context, appConfig *app.Config) (img *imgsrc.DeploymentImage, err error) {
	tb := render.NewTextBlock(ctx, "Building image")

	daemonType := imgsrc.NewDockerDaemonType(!flag.GetRemoteOnly(ctx), !flag.GetLocalOnly(ctx), env.IsCI(), flag.GetBool(ctx, "nixpacks"))

	var appName string = app.NameFromContext(ctx)
	if appConfig.AppName != "" && appName == "" {
		appName = appConfig.AppName
	}

	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	var imageRef string
	if imageRef, err = fetchImageRef(ctx, appConfig); err != nil {
		return
	}

	// we're using a pre-built Docker image
	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    appName,
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

	// We're building from source
	opts := imgsrc.ImageOptions{
		AppName:         appName,
		WorkingDir:      state.WorkingDirectory(ctx),
		Publish:         flag.GetBool(ctx, "push") || !flag.GetBuildOnly(ctx),
		ImageLabel:      flag.GetString(ctx, "image-label"),
		NoCache:         flag.GetBool(ctx, "no-cache"),
		BuiltIn:         build.Builtin,
		BuiltInSettings: build.Settings,
		Builder:         build.Builder,
		Buildpacks:      build.Buildpacks,
	}

	cliBuildSecrets, err := cmdutil.ParseKVStringsToMap(flag.GetStringSlice(ctx, "build-secret"))
	if err != nil {
		return
	}

	if cliBuildSecrets != nil {
		opts.BuildSecrets = cliBuildSecrets
	}

	var buildArgs map[string]string
	if buildArgs, err = mergeBuildArgs(ctx, build.Args); err != nil {
		return
	}

	opts.BuildArgs = buildArgs

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

func createRelease(ctx context.Context, appConfig *app.Config, img *imgsrc.DeploymentImage) (*api.Release, *api.ReleaseCommand, error) {
	tb := render.NewTextBlock(ctx, "Creating release")

	input := api.DeployImageInput{
		AppID: app.NameFromContext(ctx),
		Image: img.Tag,
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	if len(appConfig.Definition) > 0 {
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
