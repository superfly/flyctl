package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appv2"
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

func determineAppV2Config(ctx context.Context) (cfg *appv2.Config, err error) {
	tb := render.NewTextBlock(ctx, "Verifying app config")
	client := client.FromContext(ctx).API()

	appNameFromContext := determineAppName(ctx)
	appCompact, err := client.GetAppCompact(ctx, appNameFromContext)

	if err != nil {
		return nil, fmt.Errorf("failed fetching existing app config: %w", err)

	}

	if appCompact.PlatformVersion == appv2.AppsV1Platform {
		return nil, wrongAppVersionErr

	}

	if cfg = appv2.ConfigFromContext(ctx); cfg == nil {
		logger := logger.FromContext(ctx)
		logger.Debug("no local app config detected; fetching from backend ...")

		cfg, err = cmd.GetRemoteAppV2Config(ctx, client, appCompact)

		if err != nil {
			return
		}

		cfg.AppName = appCompact.Name

	} else {
		err = cfg.Validate()
		if err != nil {
			return

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

	if regionCode := flag.GetString(ctx, flag.RegionName); regionCode != "" {
		cfg.PrimaryRegion = regionCode
	}

	// Always prefer the app name passed via --app

	if appNameFromContext != "" {
		cfg.AppName = appNameFromContext
	}

	tb.Done("Verified app config")

	return
}

func DeployWithConfigV2(ctx context.Context, appConfig *appv2.Config, args DeployWithConfigArgs) (err error) {
	apiClient := client.FromContext(ctx).API()
	appNameFromContext := determineAppName(ctx)
	appCompact, err := apiClient.GetAppCompact(ctx, appNameFromContext)
	if err != nil {
		return err
	}
	deployToMachines, err := useMachinesConfigV2(appConfig, appCompact, args)
	if err != nil {
		return err
	}

	// Fetch an image ref or build from source to get the final image reference to deploy
	img, err := determineImageV2(ctx, appConfig)
	if err != nil {
		return fmt.Errorf("failed to fetch an image or build from source: %w", err)
	}

	// Assign an empty map if nil so later assignments won't fail
	if appConfig.Env == nil {
		appConfig.Env = map[string]string{}
	}

	if flag.GetBuildOnly(ctx) {
		return nil
	}

	var release *api.Release
	var releaseCommand *api.ReleaseCommand

	if appConfig.PrimaryRegion != "" && appConfig.Env["PRIMARY_REGION"] == "" {
		appConfig.Env["PRIMARY_REGION"] = appConfig.PrimaryRegion
	}

	if deployToMachines {
		ctx, err = command.LoadAppV2ConfigIfPresent(ctx)
		if err != nil {
			return fmt.Errorf("error loading appv2 config: %w", err)
		}
		primaryRegion := appConfig.PrimaryRegion
		if flag.GetString(ctx, flag.RegionName) != "" {
			primaryRegion = flag.GetString(ctx, flag.RegionName)
		}
		md, err := NewMachineDeployment(ctx, MachineDeploymentArgs{
			AppCompact:           appCompact,
			DeploymentImage:      img,
			Strategy:             flag.GetString(ctx, "strategy"),
			EnvFromFlags:         flag.GetStringSlice(ctx, "env"),
			PrimaryRegionFlag:    primaryRegion,
			AutoConfirmMigration: flag.GetBool(ctx, "auto-confirm"),
			BuildOnly:            flag.GetBuildOnly(ctx),
			SkipHealthChecks:     flag.GetDetach(ctx),
			WaitTimeout:          time.Duration(flag.GetInt(ctx, "wait-timeout")) * time.Second,
			LeaseTimeout:         time.Duration(flag.GetInt(ctx, "lease-timeout")) * time.Second,
		})
		if err != nil {
			return err
		}
		return md.DeployMachinesApp(ctx)
	}

	release, releaseCommand, err = createReleaseV2(ctx, appConfig, img)
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

		if err := watch.ReleaseCommand(ctx, appConfig.AppName, releaseCommand.ID); err != nil {
			return err
		}

		release, err = apiClient.GetAppReleaseNomad(ctx, appConfig.AppName, release.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		logger := logger.FromContext(ctx)
		logger.Debug("immediate deployment strategy, nothing to monitor")

		return nil
	}

	err = watch.Deployment(ctx, appConfig.AppName, release.EvaluationID)

	return err
}

func useMachinesConfigV2(appConfig *appv2.Config, appCompact *api.AppCompact, args DeployWithConfigArgs) (bool, error) {
	switch {
	case appCompact.PlatformVersion == appv2.AppsV2Platform:
		return true, nil
	case appCompact.Deployed:
		return appCompact.PlatformVersion == appv2.AppsV2Platform, nil
	case args.ForceNomad:
		return false, nil
	case args.ForceMachines:
		return true, nil
	case len(appConfig.Statics) > 0:
		// statics are not supported in Apps v2 yet
		return false, nil
	case args.ForceYes:
		// if running automated, stay on nomad platform for now
		return false, nil
	default:
		// choose nomad for now if not otherwise specified
		return false, nil
	}
}

func determineImageV2(ctx context.Context, appConfig *appv2.Config) (img *imgsrc.DeploymentImage, err error) {
	tb := render.NewTextBlock(ctx, "Building image")
	daemonType := imgsrc.NewDockerDaemonType(!flag.GetRemoteOnly(ctx), !flag.GetLocalOnly(ctx), env.IsCI(), flag.GetBool(ctx, "nixpacks"))

	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	resolver := imgsrc.NewResolver(daemonType, client, appConfig.AppName, io)

	var imageRef string
	if imageRef, err = fetchImageRefV2(ctx, appConfig); err != nil {
		return
	}

	// we're using a pre-built Docker image
	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    appConfig.AppName,
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
		build = new(appv2.Build)
	}

	// We're building from source
	opts := imgsrc.ImageOptions{
		AppName:         appConfig.AppName,
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

	if opts.DockerfilePath, err = resolveDockerfilePathV2(ctx, appConfig); err != nil {
		return
	}

	if opts.IgnorefilePath, err = resolveIgnorefilePathV2(ctx, appConfig); err != nil {
		return
	}

	if target := appConfig.DockerBuildTarget(); target != "" {
		opts.Target = target
	} else if target := flag.GetString(ctx, "build-target"); target != "" {
		opts.Target = target
	}

	// finally, build the image
	heartbeat := resolver.StartHeartbeat(ctx)
	defer resolver.StopHeartbeat(heartbeat)
	if img, err = resolver.BuildImage(ctx, io, opts); err == nil && img == nil {
		err = errors.New("no image specified")
	}

	if err == nil {
		tb.Printf("image: %s\n", img.Tag)
		tb.Printf("image size: %s\n", humanize.Bytes(uint64(img.Size)))
	}

	return
}

func resolveDockerfilePathV2(ctx context.Context, appConfig *appv2.Config) (path string, err error) {
	defer func() {
		if err == nil && path != "" {
			path, err = filepath.Abs(path)
		}
	}()

	if path = appConfig.Dockerfile(); path != "" {
		path = filepath.Join(filepath.Dir(appConfig.FlyTomlPath), path)
	} else {
		path = flag.GetString(ctx, "dockerfile")
	}

	return
}

func resolveIgnorefilePathV2(ctx context.Context, appConfig *appv2.Config) (path string, err error) {
	defer func() {
		if err == nil && path != "" {
			path, err = filepath.Abs(path)
		}
	}()

	if path = appConfig.Ignorefile(); path != "" {
		path = filepath.Join(filepath.Dir(appConfig.FlyTomlPath), path)
	} else {
		path = flag.GetString(ctx, "ignorefile")
	}

	return
}

func fetchImageRefV2(ctx context.Context, cfg *appv2.Config) (ref string, err error) {
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

func createReleaseV2(ctx context.Context, appConfig *appv2.Config, img *imgsrc.DeploymentImage) (*api.Release, *api.ReleaseCommand, error) {
	tb := render.NewTextBlock(ctx, "Creating release")

	input := api.DeployImageInput{
		AppID: appConfig.AppName,
		Image: img.Tag,
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	definition, err := appConfig.ToDefinition()

	if err != nil {
		return nil, nil, err

	}

	if len(*definition) > 0 {
		input.Definition = api.DefinitionPtr(*definition)
	}

	// Start deployment of the determined image
	client := client.FromContext(ctx).API()

	release, releaseCommand, err := client.DeployImage(ctx, input)
	if err == nil {
		tb.Donef("release v%d created\n", release.Version)
	}

	return release, releaseCommand, err
}