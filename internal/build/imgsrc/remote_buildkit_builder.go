package imgsrc

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
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
	defer build.BuildFinish()

	var dockerfile string

	switch {
	case opts.DockerfilePath != "" && !helpers.FileExists(opts.DockerfilePath):
		return nil, "", fmt.Errorf("dockerfile '%s' not found", opts.DockerfilePath)
	case opts.DockerfilePath != "":
		dockerfile = opts.DockerfilePath
	default:
		dockerfile = ResolveDockerfile(opts.WorkingDir)
	}

	if dockerfile == "" {
		terminal.Debug("dockerfile not found, skipping")
		return nil, "", nil
	}

	build.ImageBuildStart()
	defer build.ImageBuildFinish()

	image, err := r.buildWithRemoteBuildkit(ctx, streams, opts, dockerfile, build)
	if err != nil {
		return nil, "", err
	}
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
	defer buildState.BuilderInitFinish()
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
		return nil, fmt.Errorf("failed to connect to remote buildkit daemon at %s: %w", r.addr, retErr)
	}
	defer buildkitClient.Close()

	streams.StopProgressIndicator()

	cmdfmt.PrintDone(streams.ErrOut, fmt.Sprintf("Connected to remote buildkit daemon at %s", r.addr))

	buildState.BuildAndPushStart()
	defer buildState.BuildAndPushFinish()

	res, retErr := buildImage(ctx, buildkitClient, opts, dockerfilePath)
	if retErr != nil {
		return nil, retErr
	}

	return newDeploymentImage(res, opts.Tag)
}
