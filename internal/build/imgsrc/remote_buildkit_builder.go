package imgsrc

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var _ imageBuilder = (*RemoteBuildkitBuilder)(nil)

type RemoteBuildkitBuilder struct {
	addr string
}

func NewRemoteBuildkitBuilder(addr string) *RemoteBuildkitBuilder {
	return &RemoteBuildkitBuilder{
		addr: addr,
	}
}

func (r *RemoteBuildkitBuilder) Name() string {
	return "Remote Buildkit"
}

func (r *RemoteBuildkitBuilder) Run(ctx context.Context, _ *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "remote_buildkit_builder", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	build.BuildStart()

	var dockerfile string

	switch {
	case opts.DockerfilePath != "" && !helpers.FileExists(opts.DockerfilePath):
		build.BuildFinish()
		err := fmt.Errorf("dockerfile '%s' not found", opts.DockerfilePath)
		tracing.RecordError(span, err, "failed to find dockerfile")
		return nil, "", err
	case opts.DockerfilePath != "":
		dockerfile = opts.DockerfilePath
	default:
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
			tracing.RecordError(span, err, "failed to get relative dockerfile path")
			build.BuildFinish()
			return nil, "", err
		}
		// On Windows, convert \ to a slash / as the docker build will
		// run in a Linux VM at the end.
		relDockerfile = filepath.ToSlash(p)
	}
	span.SetAttributes(attribute.String("relative_dockerfile_path", relDockerfile))

	build.ImageBuildStart()

	image, err := r.buildWithRemoteBuildkit(ctx, streams, opts, dockerfile, build)
	if err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		tracing.RecordError(span, err, "failed to build image")
		return nil, "", errors.Wrap(err, "error building")
	}

	build.ImageBuildFinish()
	build.BuildFinish()
	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	span.SetAttributes(image.ToSpanAttributes()...)
	return image, "", nil
}

func (r *RemoteBuildkitBuilder) buildWithRemoteBuildkit(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions, dockerfilePath string, buildState *build) (i *DeploymentImage, retErr error) {
	ctx, span := tracing.GetTracer().Start(ctx, "remote_buildkit_build", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer func() {
		if retErr != nil {
			streams.StopProgressIndicator()
			span.RecordError(retErr)
		}
		span.End()
	}()

	buildState.BuilderInitStart()
	buildState.SetBuilderMetaPart1("remote-buildkit", r.addr, "")

	{
		msg := fmt.Sprintf("Connecting to remote buildkit daemon at %s...\n", r.addr)
		if streams.IsInteractive() {
			streams.StartProgressIndicatorMsg(msg)
		} else {
			fmt.Fprintln(streams.ErrOut, msg)
		}
	}

	span.AddEvent("connecting to buildkit")
	var buildkitClient *client.Client
	buildkitClient, retErr = client.New(ctx, r.addr)
	if retErr != nil {
		buildState.BuilderInitFinish()
		return nil, fmt.Errorf("failed to connect to remote buildkit daemon at %s: %w", r.addr, retErr)
	}
	defer buildkitClient.Close()

	buildState.BuilderInitFinish()
	streams.StopProgressIndicator()

	cmdfmt.PrintDone(streams.ErrOut, fmt.Sprintf("Connected to remote buildkit daemon at %s", r.addr))

	buildState.BuildAndPushStart()
	res, retErr := buildImage(ctx, buildkitClient, opts, dockerfilePath)
	if retErr != nil {
		buildState.BuildAndPushFinish()
		return nil, retErr
	}
	buildState.BuildAndPushFinish()

	return newDeploymentImage(res, opts.Tag)
}
