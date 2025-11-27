package imgsrc

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/containerd/containerd/api/services/content/v1"
	"github.com/moby/buildkit/client"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/trace"
)

var _ imageBuilder = (*BuildkitBuilder)(nil)

type BuildkitBuilder struct {
	// addr is the address of the Buildkit daemon.
	// The client may need a WireGuard connection to reach the address.
	addr string

	// provisioner is used to provision a builder machine if needed.
	provisioner *Provisioner
}

// NewBuildkitBuilder creates a builder that directly uses Buildkit instead of Docker Engine.
// addr is the address of the deamon (e.g. "foobar.flycast:1234" which is optional).
func NewBuildkitBuilder(addr string, provisioner *Provisioner) *BuildkitBuilder {
	if !provisioner.UseBuildkit() {
		panic("provisioner must be configured to use Buildkit")
	}

	return &BuildkitBuilder{addr: addr, provisioner: provisioner}
}

func (r *BuildkitBuilder) Name() string { return "Buildkit" }

func (r *BuildkitBuilder) Run(ctx context.Context, _ *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "buildkit_builder", trace.WithAttributes(opts.ToSpanAttributes()...))
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

	image, err := r.buildWithBuildkit(ctx, streams, opts, dockerfile, build)
	if err != nil {
		return nil, "", err
	}
	build.BuilderMeta.RemoteMachineId = image.BuilderID
	cmdfmt.PrintDone(streams.ErrOut, "Building image done")
	span.SetAttributes(image.ToSpanAttributes()...)
	return image, "", nil
}

func (r *BuildkitBuilder) buildWithBuildkit(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions, dockerfilePath string, buildState *build) (i *DeploymentImage, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "buildkit_build", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		streams.StopProgressIndicator()
		span.End()
	}()

	app := r.provisioner.org.RemoteBuilderApp
	if r.addr == "" && app != nil {
		r.addr = fmt.Sprintf("%s.flycast:%d", app.Name, buildkitGRPCPort)
	}

	buildState.BuilderInitStart()
	defer buildState.BuilderInitFinish()
	buildState.SetBuilderMetaPart1("buildkit", r.addr, "")

	streams.StartProgressIndicator()

	buildkitClient, err := r.connectClient(ctx, appToAppCompact(app), opts.AppName)
	if err != nil {
		return nil, fmt.Errorf("failed to create buildkit client: %w", err)
	}

	streams.StopProgressIndicator()
	cmdfmt.PrintDone(streams.ErrOut, fmt.Sprintf("Connected to buildkit daemon at %s", r.addr))

	buildState.BuildAndPushStart()
	defer buildState.BuildAndPushFinish()

	res, err := buildImage(ctx, buildkitClient, opts, dockerfilePath)
	if err != nil {
		return nil, err
	}

	return newDeploymentImage(ctx, buildkitClient, res, opts.Tag)
}

func (r *BuildkitBuilder) connectClient(ctx context.Context, app *fly.AppCompact, appName string) (*client.Client, error) {
	recreateBuilder := flag.GetRecreateBuilder(ctx)
	ensureBuilder := false
	if r.addr == "" || recreateBuilder {
		updateProgress(ctx, "Updating remote builder...")
		_, builderApp, err := r.provisioner.EnsureBuilder(
			ctx, os.Getenv("FLY_REMOTE_BUILDER_REGION"), recreateBuilder,
		)
		if err != nil {
			return nil, err
		}
		app = appToAppCompact(builderApp)
		r.addr = fmt.Sprintf("%s.flycast:%d", app.Name, buildkitGRPCPort)
		ensureBuilder = true
	}
	var opts []client.ClientOpt
	apiClient := flyutil.ClientFromContext(ctx)
	if app != nil {
		_, dialer, err := agent.BringUpAgent(ctx, apiClient, app, app.Network, true)
		if err != nil {
			return nil, fmt.Errorf("failed wireguard connection: %w", err)
		}
		opts = append(opts, client.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", addr)
		}))
	}

	updateProgress(ctx, "Connecting to buildkit daemon at %s...", r.addr)
	buildkitClient, err := client.New(ctx, r.addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create buildkit client: %w", err)
	}
	_, err = buildkitClient.Info(ctx)
	if err != nil {
		if app == nil { // Retry with Wireguard connection
			app, err = apiClient.GetAppCompact(ctx, appName)
			if err != nil {
				return nil, fmt.Errorf("failed to get app: %w", err)
			}
			return r.connectClient(ctx, app, appName)
		} else if !ensureBuilder && r.provisioner.buildkitImage != "" { // Retry with ensureBuilder
			r.addr = ""
			return r.connectClient(ctx, nil, appName)
		} else {
			return nil, fmt.Errorf("failed to connect to buildkit: %w", err)
		}
	}
	return buildkitClient, nil
}

func updateProgress(ctx context.Context, msg string, a ...any) {
	msg = fmt.Sprintf(msg+"\n", a...)
	streams := iostreams.FromContext(ctx)
	if streams.IsInteractive() {
		streams.ChangeProgressIndicatorMsg(msg)
	} else {
		fmt.Fprintln(streams.ErrOut, msg)
	}
}

func readContent(ctx context.Context, contentClient content.ContentClient, desc *Descriptor) (string, error) {
	readClient, err := contentClient.Read(ctx, &content.ReadContentRequest{Digest: desc.Digest})
	if err != nil {
		return "", fmt.Errorf("failed to create read stream: %w", err)
	}
	var data []byte
	for {
		resp, err := readClient.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("failed to read from stream: %w", err)
		}
		data = append(data, resp.Data...)
	}
	return string(data), nil
}
