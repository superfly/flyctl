package imgsrc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/console"
	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
)

type dockerfileBuilder struct{}

func (*dockerfileBuilder) Name() string {
	return "Dockerfile"
}

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
			return nil, "", fmt.Errorf("Dockerfile '%s' not found", opts.DockerfilePath)
		}
		dockerfile = opts.DockerfilePath
	} else {
		dockerfile = ResolveDockerfile(opts.WorkingDir)
	}

	if dockerfile == "" {
		terminal.Debug("dockerfile not found, skipping")
		build.BuildFinish()
		return nil, "", nil
	}

	var relDockerfile string
	if isPathInRoot(dockerfile, opts.WorkingDir) {
		// pass the relative path to Dockerfile within the context
		p, err := filepath.Rel(opts.WorkingDir, dockerfile)
		if err != nil {
			return nil, "", err
		}
		// On Windows, convert \ to a slash / as the docker build will
		// run in a Linux VM at the end.
		relDockerfile = filepath.ToSlash(p)
	}

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
		return nil, "", fmt.Errorf("error checking for buildkit support: %w", err)
	}

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
	serverInfo, err := func() (types.Info, error) {
		infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return docker.Info(infoCtx)
	}()
	if err != nil {
		if dockerFactory.IsRemote() {
			metrics.SaveNoData(ctx, "remote_builder_failure")
		}
		build.ImageBuildFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error fetching docker server info")
	}

	docker_tb := render.NewTextBlock(ctx, "Building image with Docker")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	docker_tb.Done(msg)

	buildArgs, err := normalizeBuildArgsForDocker(opts.BuildArgs)
	if err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		return nil, "", fmt.Errorf("error parsing build args: %w", err)
	}

	build.SetBuilderMetaPart2(buildkitEnabled, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
	if buildkitEnabled {
		imageID, err = runBuildKitBuild(ctx, docker, opts, dockerfile, buildArgs)
		if err != nil {
			if dockerFactory.IsRemote() {
				metrics.SaveNoData(ctx, "remote_builder_failure")
			}
			build.ImageBuildFinish()
			build.BuildFinish()
			return nil, "", errors.Wrap(err, "error building")
		}
	} else {
		imageID, err = runClassicBuild(ctx, streams, docker, buildContext, opts, relDockerfile, buildArgs)
		if err != nil {
			if dockerFactory.IsRemote() {
				metrics.SaveNoData(ctx, "remote_builder_failure")
			}
			build.ImageBuildFinish()
			build.BuildFinish()
			return nil, "", errors.Wrap(err, "error building")
		}
	}

	build.ImageBuildFinish()
	build.BuildFinish()
	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	if opts.Publish {
		build.PushStart()
		tb := render.NewTextBlock(ctx, "Pushing image to fly")
		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			build.PushFinish()
			return nil, "", err
		}
		build.PushFinish()

		tb.Done("Pushing image done")
	}

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, "", errors.Wrap(err, "count not find built image")
	}

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, "", nil
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
	options := types.ImageBuildOptions{
		Tags:        []string{opts.Tag},
		BuildArgs:   buildArgs,
		AuthConfigs: authConfigs(config.FromContext(ctx).AccessToken),
		Platform:    "linux/amd64",
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

func solveOptFromImageOptions(opts ImageOptions, dockerfilePath string, buildArgs map[string]*string) client.SolveOpt {
	attrs := map[string]string{
		"filename": filepath.Base(dockerfilePath),
		"target":   opts.Target,
		// Fly.io only supports linux/amd64, but local Docker Engine could be running on ARM,
		// including Apple Silicon.
		"platform": "linux/amd64",
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

	return client.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: attrs,
		LocalDirs: map[string]string{
			"dockerfile": filepath.Dir(dockerfilePath),
			"context":    opts.WorkingDir,
		},
		// Docker Engine's worker only supports three exporters.
		// "moby" exporter works best for flyctl, since we want to keep images in
		// Docker Engine's image store. The others are exporting images to somewhere else.
		// https://github.com/moby/moby/blob/v20.10.24/builder/builder-next/worker/worker.go#L221
		Exports: []client.ExportEntry{
			{Type: "moby", Attrs: map[string]string{"name": opts.Tag}},
		},
	}
}

func runBuildKitBuild(ctx context.Context, docker *dockerclient.Client, opts ImageOptions, dockerfilePath string, buildArgs map[string]*string) (string, error) {
	// Connect to Docker Engine's embedded Buildkit.
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return docker.DialHijack(ctx, "/grpc", "h2c", map[string][]string{})
	}
	bc, err := client.New(ctx, "", client.WithContextDialer(dialer), client.WithFailFast)
	if err != nil {
		return "", err
	}

	// Build the image.
	statusCh := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var (
			con console.Console
			err error
		)
		// On GitHub Actions, os.Stderr is not console.
		// https://community.fly.io/t/error-failed-to-fetch-an-image-or-build-from-source-error-building-provided-file-is-not-a-console/14273
		con, err = console.ConsoleFromFile(os.Stderr)
		if err != nil {
			// It should be nil, but just in case.
			con = nil
		}
		// Don't use `ctx` here.
		// Cancelling the context kills the reader of statusCh which blocks bc.Solve below.
		// bc.Solve closes statusCh at the end and DisplaySolveStatus returns by reading the closed channel.
		_, err = progressui.DisplaySolveStatus(context.Background(), "", con, os.Stdout, statusCh)
		return err
	})
	var res *client.SolveResponse
	eg.Go(func() error {
		options := solveOptFromImageOptions(opts, dockerfilePath, buildArgs)
		secrets := make(map[string][]byte)
		for k, v := range opts.BuildSecrets {
			secrets[k] = []byte(v)
		}
		options.Session = append(
			options.Session,
			// To pull images from local Docker Engine with Fly's access token,
			// we need to pass the provider. Remote builders don't need that.
			newBuildkitAuthProvider(config.FromContext(ctx).AccessToken),
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

	metrics.Started(ctx, "image_push")
	sendImgPushMetrics := metrics.StartTiming(ctx, "image_push/duration")

	pushResp, err := docker.ImagePush(ctx, tag, types.ImagePushOptions{
		RegistryAuth: flyRegistryAuth(config.FromContext(ctx).AccessToken),
	})
	metrics.Status(ctx, "image_push", err == nil)

	if err != nil {
		return errors.Wrap(err, "error pushing image to registry")
	}
	defer pushResp.Close() // skipcq: GO-S2307
	sendImgPushMetrics()

	err = jsonmessage.DisplayJSONMessagesStream(pushResp, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), nil)
	if err != nil {
		var msgerr *jsonmessage.JSONError

		if errors.As(err, &msgerr) {
			if msgerr.Message == "denied: requested access to the resource is denied" {
				return &RegistryUnauthorizedError{Tag: tag}
			}
		}
		return errors.Wrap(err, "error rendering push status stream")
	}

	return nil
}
