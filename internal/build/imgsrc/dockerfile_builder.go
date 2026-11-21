package imgsrc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/system"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"github.com/tonistiigi/fsutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type dockerfileBuilder struct{}

func (*dockerfileBuilder) Name() string {
	return "Dockerfile"
}

func (*dockerfileBuilder) usesDockerfile() {}

// lastProgressOutput is the same as progress.Output except
// that it only output with the last update. It is used in
// non terminal scenarios to suppress verbose messages
type lastProgressOutput struct {
	output progress.Output
}

// WriteProgress formats progress information from a ProgressReader.
func (out *lastProgressOutput) WriteProgress(prog progress.Progress) error {
	if !prog.LastUpdate {
		return nil
	}

	return out.output.WriteProgress(prog)
}

func makeBuildContext(dockerfile string, opts ImageOptions, isRemote bool) (io.ReadCloser, error) {
	archiveOpts := archiveOptions{
		sourcePath: opts.WorkingDir,
		compressed: isRemote,
	}

	var relativedockerfilePath string

	// copy dockerfile into the archive if it's outside the context dir
	if !isPathInRoot(dockerfile, opts.WorkingDir) {
		dockerfileData, err := os.ReadFile(dockerfile)
		if err != nil {
			return nil, errors.Wrap(err, "error reading Dockerfile")
		}
		archiveOpts.additions = map[string][]byte{
			"Dockerfile": dockerfileData,
		}
	} else {
		// pass the relative path to Dockerfile within the context
		p, err := filepath.Rel(opts.WorkingDir, dockerfile)
		if err != nil {
			return nil, err
		}
		// On Windows, convert \ to a slash / as the docker build will
		// run in a Linux VM at the end.
		relativedockerfilePath = filepath.ToSlash(p)
	}

	excludes, err := readDockerignore(opts.WorkingDir, opts.IgnorefilePath, relativedockerfilePath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading .dockerignore")
	}
	archiveOpts.exclusions = excludes

	// Start tracking this build

	// Create the docker build context as a compressed tar stream
	r, err := archiveDirectory(archiveOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error archiving build context")
	}

	return r, nil
}

func (*dockerfileBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "dockerfile_builder", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	build.BuildStart()
	if !dockerFactory.mode.IsAvailable() {
		// Where should debug messages be sent?
		terminal.Debug("docker daemon not available, skipping")
		build.BuildFinish()

		return nil, "", nil
	}

	var dockerfile string

	if opts.DockerfilePath != "" {
		if !helpers.FileExists(opts.DockerfilePath) {
			build.BuildFinish()
			err := fmt.Errorf("dockerfile '%s' not found", opts.DockerfilePath)
			tracing.RecordError(ctx, span, err, "failed to find dockerfile")

			return nil, "", err
		}
		dockerfile = opts.DockerfilePath
	} else {
		dockerfile = ResolveDockerfile(opts.WorkingDir)
	}

	if dockerfile == "" {
		span.AddEvent("dockerfile not found, skipping")
		terminal.Debug("dockerfile not found, skipping")
		build.BuildFinish()

		return nil, "", nil
	}

	var relDockerfile string
	if isPathInRoot(dockerfile, opts.WorkingDir) {
		// pass the relative path to Dockerfile within the context
		p, err := filepath.Rel(opts.WorkingDir, dockerfile)
		if err != nil {
			tracing.RecordError(ctx, span, err, "failed to get relative dockerfile path")

			return nil, "", err
		}
		// On Windows, convert \ to a slash / as the docker build will
		// run in a Linux VM at the end.
		relDockerfile = filepath.ToSlash(p)
	}
	span.SetAttributes(attribute.String("relative_dockerfile_path", relDockerfile))

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	if err != nil {
		build.BuildFinish()
		build.BuilderInitFinish()

		return nil, "", errors.Wrap(err, "error connecting to docker")
	}
	defer docker.Close() // skipcq: GO-S2307

	buildkitEnabled, err := buildkitEnabled(docker)
	terminal.Debugf("buildkitEnabled %v", buildkitEnabled)
	if err != nil {
		build.BuildFinish()
		build.BuilderInitFinish()
		tracing.RecordError(ctx, span, err, "failed to check for buildkit support")

		return nil, "", fmt.Errorf("error checking for buildkit support: %w", err)
	}

	span.SetAttributes(attribute.Bool("buildkit_enabled", buildkitEnabled))

	build.BuilderInitFinish()
	defer func() {
		// Don't untag images for remote builder, as people sometimes
		// run concurrent builds from CI that end up racing with each other
		// and one of them failing with 404 while calling docker.ImageInspectWithRaw
		if dockerFactory.IsLocal() {
			clearDeploymentTags(ctx, docker, opts.Tag)
		}
	}()

	// Without Buildkit, we need to explicitly build a build context beforehand.
	var buildContext io.ReadCloser
	if !buildkitEnabled {
		build.ContextBuildStart()

		tb := render.NewTextBlock(ctx, "Creating build context")

		r, err := makeBuildContext(dockerfile, opts, dockerFactory.IsRemote())
		if err != nil {
			build.BuildFinish()
			build.ContextBuildFinish()
			tracing.RecordError(ctx, span, err, "failed to make build context")

			return nil, "", err
		}

		tb.Done("Creating build context done")

		build.ContextBuildFinish()

		// Setup an upload progress bar
		progressOutput := streamformatter.NewProgressOutput(streams.Out)
		if !streams.IsStdoutTTY() {
			progressOutput = &lastProgressOutput{output: progressOutput}
		}

		buildContext = progress.NewProgressReader(r, progressOutput, 0, "", "Sending build context to Docker daemon")
	}

	var imageID string

	build.ImageBuildStart()
	terminal.Debug("fetching docker server info")
	serverInfo, err := func() (system.Info, error) {
		infoCtx, cancel := context.WithTimeoutCause(ctx, 10*time.Second, fmt.Errorf("fetching Docker server info: %w", context.DeadlineExceeded))
		defer cancel()

		info, err := docker.Info(infoCtx)
		if err != nil {
			tracing.RecordCancellation(infoCtx, span)
		}

		return info, err
	}()
	if err != nil {
		if dockerFactory.IsRemote() {
			metrics.SendNoData(ctx, "remote_builder_failure")
		}
		build.ImageBuildFinish()
		build.BuildFinish()
		tracing.RecordError(ctx, span, err, "failed to fetch docker server info")

		return nil, "", errors.Wrap(err, "error fetching docker server info")
	}

	docker_tb := render.NewTextBlock(ctx, "Building image with Docker")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	docker_tb.Done(msg)

	buildArgs, err := normalizeBuildArgsForDocker(opts.BuildArgs)
	if err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		tracing.RecordError(ctx, span, err, "failed to parse build args")

		return nil, "", fmt.Errorf("error parsing build args: %w", err)
	}

	build.SetBuilderMetaPart2(buildkitEnabled, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
	if buildkitEnabled {
		imageID, err = runBuildKitBuild(ctx, docker, opts, dockerfile, buildArgs)
		if err != nil {
			if dockerFactory.IsRemote() {
				metrics.SendNoData(ctx, "remote_builder_failure")
			}
			build.ImageBuildFinish()
			build.BuildFinish()
			tracing.RecordError(ctx, span, err, "failed to build image")

			return nil, "", errors.Wrap(err, "error building")
		}
	} else {
		imageID, err = runClassicBuild(ctx, streams, docker, buildContext, opts, relDockerfile, buildArgs)
		if err != nil {
			if dockerFactory.IsRemote() {
				metrics.SendNoData(ctx, "remote_builder_failure")
			}
			build.ImageBuildFinish()
			build.BuildFinish()
			tracing.RecordError(ctx, span, err, "failed to build image")

			return nil, "", errors.Wrap(err, "error building")
		}
	}

	build.ImageBuildFinish()
	build.BuildFinish()
	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	var pushedDigest digest.Digest
	if opts.Publish {
		build.PushStart()
		tb := render.NewTextBlock(ctx, "Pushing image to fly")
		pushedDigest, err = pushToFlyWithDigest(ctx, docker, streams, opts.Tag)
		if err != nil {
			build.PushFinish()

			return nil, "", err
		}
		build.PushFinish()

		tb.Done("Pushing image done")
	}

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if errdefs.IsNotFound(err) && opts.Publish && dockerFactory.apiClient != nil {
		recovered, lookupErr := resolveMissingBuildImage(ctx, dockerFactory.apiClient, opts, streams.ErrOut, imageID, pushedDigest, buildkitEnabled)
		if lookupErr != nil {
			return nil, "", fmt.Errorf("local image %s is missing: %w", imageID, lookupErr)
		}
		if recovered != nil {
			span.SetAttributes(recovered.ToSpanAttributes()...)

			return recovered, "", nil
		}
	}
	if err != nil {
		return nil, "", errors.Wrap(err, "count not find built image")
	}

	di := DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}

	span.SetAttributes(di.ToSpanAttributes()...)

	return &di, "", nil
}

// A concurrent build can remove our local image after the push. Use the
// manifest in Docker's push receipt, or, for exporters without a receipt,
// require the pushed tag to still match the BuildKit result.
func resolveMissingBuildImage(ctx context.Context, api flyClient, opts ImageOptions, out io.Writer, imageID string, pushed digest.Digest, buildkit bool) (*DeploymentImage, error) {
	lookup := opts.Tag
	if pushed != "" {
		lookup += "@" + pushed.String()
	} else if buildkit {
		pushed, _ = digest.Parse(imageID)
	}
	if pushed == "" {
		return nil, nil
	}

	return resolveVerifiedBuildImage(ctx, api, opts, out, lookup, imageID, pushed)
}

func resolveVerifiedBuildImage(ctx context.Context, api flyClient, opts ImageOptions, out io.Writer, lookup, imageID string, pushed digest.Digest) (*DeploymentImage, error) {
	img, err := resolveImageWithRetry(ctx, api, opts.AppName, lookup, defaultImageWait, out)
	if err != nil || img == nil {
		return nil, err
	}
	if img.Digest != pushed.String() {
		return nil, nil
	}
	size, err := strconv.ParseInt(img.CompressedSize, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid registry image size: %w", err)
	}

	if imageID == "" {
		imageID = img.ID
	}
	return &DeploymentImage{ID: imageID, Tag: opts.Tag + "@" + img.Digest, Digest: img.Digest, Size: size}, nil
}

func normalizeBuildArgsForDocker(buildArgs map[string]string) (map[string]*string, error) {
	out := map[string]*string{}

	for k, v := range buildArgs {
		val := v
		out[k] = &val
	}

	return out, nil
}

func runClassicBuild(ctx context.Context, streams *iostreams.IOStreams, docker *dockerclient.Client, r io.ReadCloser, opts ImageOptions, dockerfilePath string, buildArgs map[string]*string) (imageID string, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "build_image",
		trace.WithAttributes(opts.ToSpanAttributes()...),
		trace.WithAttributes(attribute.String("type", "classic")),
	)
	defer span.End()

	platform := "linux/amd64"
	if p := os.Getenv("FLY_DEV_PLATFORM"); p != "" {
		platform = p
	}

	options := types.ImageBuildOptions{
		Tags:        []string{opts.Tag},
		BuildArgs:   buildArgs,
		AuthConfigs: authConfigs(config.Tokens(ctx).Docker()),
		Platform:    platform,
		Dockerfile:  dockerfilePath,
		Target:      opts.Target,
		NoCache:     opts.NoCache,
		Labels:      opts.Label,
	}

	resp, err := docker.ImageBuild(ctx, r, options)
	if err != nil {
		return "", errors.Wrap(err, "error building with docker")
	}
	defer resp.Body.Close() // skipcq: GO-S2307

	idCallback := func(m jsonmessage.JSONMessage) {
		var aux types.BuildResult
		if err := json.Unmarshal(*m.Aux, &aux); err != nil {
			fmt.Fprintf(streams.Out, "failed to parse aux message: %v", err)
		}
		imageID = aux.ID
	}

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), idCallback); err != nil {
		return "", errors.Wrap(err, "error rendering build status stream")
	}

	return imageID, nil
}

func solveOptFromImageOptions(opts ImageOptions, dockerfilePath string, buildArgs map[string]*string) (client.SolveOpt, error) {
	// Fly.io only supports linux/amd64, but local Docker Engine could be running on ARM,
	// including Apple Silicon. Use FLY_DEV_PLATFORM to override for local testing.
	platform := "linux/amd64"
	if p := os.Getenv("FLY_DEV_PLATFORM"); p != "" {
		platform = p
	}

	attrs := map[string]string{
		"filename": filepath.Base(dockerfilePath),
		"target":   opts.Target,
		"platform": platform,
	}
	attrs["target"] = opts.Target
	if opts.NoCache {
		attrs["no-cache"] = ""
	}

	for k, v := range opts.Label {
		attrs["label:"+k] = v
	}

	for k, v := range buildArgs {
		if v == nil {
			continue
		}
		attrs["build-arg:"+k] = *v
	}

	dockerfileDir, err := fsutil.NewFS(filepath.Dir(dockerfilePath))
	if err != nil {
		return client.SolveOpt{}, err
	}
	contextDir, err := fsutil.NewFS(opts.WorkingDir)
	if err != nil {
		return client.SolveOpt{}, err
	}

	return client.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: attrs,
		LocalMounts: map[string]fsutil.FS{
			"dockerfile": dockerfileDir,
			"context":    contextDir,
		},
		// Docker Engine's worker only supports three exporters.
		// "moby" exporter works best for flyctl, since we want to keep images in
		// Docker Engine's image store. The others are exporting images to somewhere else.
		// https://github.com/moby/moby/blob/v20.10.24/builder/builder-next/worker/worker.go#L221
		Exports: []client.ExportEntry{
			{Type: "moby", Attrs: map[string]string{"name": opts.Tag}},
		},
	}, nil
}

func runBuildKitBuild(ctx context.Context, docker *dockerclient.Client, opts ImageOptions, dockerfilePath string, buildArgs map[string]*string) (string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "build_image",
		trace.WithAttributes(opts.ToSpanAttributes()...),
		trace.WithAttributes(attribute.String("type", "buildkit")),
	)
	defer span.End()

	// Connect to Docker Engine's embedded Buildkit.
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return dialDockerBuildkit(ctx, docker)
	}
	bc, err := client.New(ctx, "", client.WithContextDialer(dialer))
	if err != nil {
		return "", err
	}

	// Build the image.
	statusCh := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(newDisplay(statusCh))
	var res *client.SolveResponse
	eg.Go(func() error {
		options, err := solveOptFromImageOptions(opts, dockerfilePath, buildArgs)
		if err != nil {
			return err
		}
		secrets := make(map[string][]byte)
		for k, v := range opts.BuildSecrets {
			secrets[k] = []byte(v)
		}
		options.Session = append(
			options.Session,
			// To pull images from local Docker Engine with Fly's access token,
			// we need to pass the provider. Remote builders don't need that.
			newBuildkitAuthProvider(func() string {
				return config.Tokens(ctx).Docker()
			}),
			secretsprovider.FromMap(secrets),
		)

		res, err = bc.Solve(ctx, nil, options, statusCh)
		if err != nil {
			return err
		}

		return nil
	})
	err = eg.Wait()

	if err != nil {
		return "", err
	}

	return res.ExporterResponse[exptypes.ExporterImageDigestKey], nil
}

func pushToFly(ctx context.Context, docker *dockerclient.Client, streams *iostreams.IOStreams, tag string) error {
	_, err := pushToFlyWithDigest(ctx, docker, streams, tag)

	return err
}

func pushToFlyWithDigest(ctx context.Context, docker *dockerclient.Client, streams *iostreams.IOStreams, tag string) (pushed digest.Digest, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "push_image_to_registry", trace.WithAttributes(attribute.String("tag", tag)))
	defer span.End()

	defer func() {
		if err != nil {
			tracing.RecordError(ctx, span, err, "failed to push to fly registry")
		}
	}()

	metrics.Started(ctx, "image_push")
	sendImgPushMetrics := metrics.StartTiming(ctx, "image_push/duration")

	pushResp, err := docker.ImagePush(ctx, tag, image.PushOptions{
		RegistryAuth: flyRegistryAuth(config.Tokens(ctx).Docker()),
	})
	metrics.Status(ctx, "image_push", err == nil)

	if err != nil {
		return "", errors.Wrap(err, "error pushing image to registry")
	}
	defer pushResp.Close() // skipcq: GO-S2307
	sendImgPushMetrics()

	err = jsonmessage.DisplayJSONMessagesStream(pushResp, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), func(m jsonmessage.JSONMessage) {
		var result types.PushResult
		if json.Unmarshal(*m.Aux, &result) == nil && result.Tag != "" && strings.HasSuffix(tag, ":"+result.Tag) {
			if d, err := digest.Parse(result.Digest); err == nil {
				pushed = d
			}
		}
	})
	if err != nil {
		var msgerr *jsonmessage.JSONError

		if errors.As(err, &msgerr) {
			if msgerr.Message == "denied: requested access to the resource is denied" {
				return "", &RegistryUnauthorizedError{Tag: tag}
			}
		}

		return "", errors.Wrap(err, "error rendering push status stream")
	}

	return pushed, nil
}
