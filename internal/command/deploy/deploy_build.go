package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

func multipleDockerfile(ctx context.Context, appConfig *appconfig.Config) error {
	if len(appConfig.BuildStrategies()) == 0 {
		// fly.toml doesn't know anything about building this image.
		return nil
	}

	found := imgsrc.ResolveDockerfile(state.WorkingDirectory(ctx))
	if found == "" {
		// No Dockerfile in the directory.
		return nil
	}

	config, _ := resolveDockerfilePath(ctx, appConfig)
	if config == "" {
		// No Dockerfile in fly.toml.
		return nil
	}

	if found != config {
		return fmt.Errorf("Ignoring %s, and using %s (from fly.toml).", found, config)
	}
	return nil
}

// determineImage picks the deployment strategy, builds the image and returns a
// DeploymentImage struct
func determineImage(ctx context.Context, appConfig *appconfig.Config) (img *imgsrc.DeploymentImage, err error) {
	tb := render.NewTextBlock(ctx, "Building image")
	daemonType := imgsrc.NewDockerDaemonType(!flag.GetRemoteOnly(ctx), !flag.GetLocalOnly(ctx), env.IsCI(), flag.GetBool(ctx, "nixpacks"))

	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	if err := multipleDockerfile(ctx, appConfig); err != nil {
		terminal.Warnf("%s\n", err.Error())
	}

	resolver := imgsrc.NewResolver(daemonType, client, appConfig.AppName, io)

	var imageRef string
	if imageRef, err = fetchImageRef(ctx, appConfig); err != nil {
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
		build = new(appconfig.Build)
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

	// flyctl supports key=value form while Docker supports id=key,src=/path/to/secret form.
	// https://docs.docker.com/engine/reference/commandline/buildx_build/#secret
	cliBuildSecrets, err := cmdutil.ParseKVStringsToMap(flag.GetStringArray(ctx, "build-secret"))
	if err != nil {
		return
	}

	if cliBuildSecrets != nil {
		opts.BuildSecrets = cliBuildSecrets
	}

	arrLabels := flag.GetStringArray(ctx, "label")
	labels, err := cmdutil.ParseKVStringsToMap(arrLabels)
	if err != nil {
		return
	}
	if env.IS_GH_ACTION() {
		labels["GH_SHA"] = env.GitCommitSHA()
		labels["GH_ACTION_NAME"] = env.GitActionName()
		labels["GH_REPO"] = env.GitRepoAndOwner()
		labels["GH_EVENT_NAME"] = env.GitActionEventName()
	}
	if labels != nil {
		opts.Label = labels
	}

	var buildArgs map[string]string
	if buildArgs, err = mergeBuildArgs(ctx, build.Args); err != nil {
		return
	}

	opts.BuildArgs = buildArgs

	if opts.DockerfilePath, err = resolveDockerfilePath(ctx, appConfig); err != nil {
		return
	}

	if opts.IgnorefilePath, err = resolveIgnorefilePath(ctx, appConfig); err != nil {
		return
	}

	if target := appConfig.DockerBuildTarget(); target != "" {
		opts.Target = target
	} else if target := flag.GetString(ctx, "build-target"); target != "" {
		opts.Target = target
	}

	// finally, build the image
	heartbeat, err := resolver.StartHeartbeat(ctx)
	if err != nil {
		metrics.SendNoData(ctx, "remote_builder_failure")
		return nil, err
	}
	defer heartbeat.Stop()

	metrics.Started(ctx, "remote_build_image")
	sendDurationMetrics := metrics.StartTiming(ctx, "remote_build_image/duration")

	if img, err = resolver.BuildImage(ctx, io, opts); err == nil && img == nil {
		err = errors.New("no image specified")
	}
	metrics.Status(ctx, "remote_build_image", err == nil)
	if err == nil {
		sendDurationMetrics()
	}

	if err == nil {
		tb.Printf("image: %s\n", img.Tag)
		tb.Printf("image size: %s\n", humanize.Bytes(uint64(img.Size)))
	}

	return
}

// resolveDockerfilePath returns the absolute path to the Dockerfile
// if one was specified in the app config or a command line argument
func resolveDockerfilePath(ctx context.Context, appConfig *appconfig.Config) (path string, err error) {
	defer func() {
		if err == nil && path != "" {
			path, err = filepath.Abs(path)
		}
	}()

	if path = appConfig.Dockerfile(); path != "" {
		path = filepath.Join(filepath.Dir(appConfig.ConfigFilePath()), path)
	} else {
		path = flag.GetString(ctx, "dockerfile")
	}

	return
}

// resolveIgnorefilePath returns the absolute path to the Dockerfile
// if one was specified in the app config or a command line argument
func resolveIgnorefilePath(ctx context.Context, appConfig *appconfig.Config) (path string, err error) {
	defer func() {
		if err == nil && path != "" {
			path, err = filepath.Abs(path)
		}
	}()

	if path = appConfig.Ignorefile(); path != "" {
		path = filepath.Join(filepath.Dir(appConfig.ConfigFilePath()), path)
	} else {
		path = flag.GetString(ctx, "ignorefile")
	}

	return
}

func mergeBuildArgs(ctx context.Context, args map[string]string) (map[string]string, error) {
	if args == nil {
		args = make(map[string]string)
	}

	// set additional Docker build args from the command line, overriding similar ones from the config
	cliBuildArgs, err := cmdutil.ParseKVStringsToMap(flag.GetStringArray(ctx, "build-arg"))
	if err != nil {
		return nil, fmt.Errorf("invalid build args: %w", err)
	}

	for k, v := range cliBuildArgs {
		args[k] = v
	}
	return args, nil
}

func fetchImageRef(ctx context.Context, cfg *appconfig.Config) (ref string, err error) {
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
