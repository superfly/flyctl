package imgsrc

import (
	"context"
	"fmt"
	"io"
	"os"

	packclient "github.com/buildpacks/pack/pkg/client"
	projectTypes "github.com/buildpacks/pack/pkg/project/types"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type buildpacksBuilder struct{}

func (*buildpacksBuilder) Name() string {
	return "Buildpacks"
}

func returnTrue(s string) bool {
	return true
}

func (*buildpacksBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "buildpacks_builder", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	build.BuildStart()
	if !dockerFactory.mode.IsAvailable() {
		note := "docker daemon not available, skipping"
		span.AddEvent(note)
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	if opts.Builder == "" {
		note := "no buildpack builder configured, skipping"
		terminal.Debug(note)
		span.AddEvent(note)
		build.BuildFinish()
		return nil, note, nil
	}

	builder := opts.Builder
	buildpacks := opts.Buildpacks

	span.SetAttributes(attribute.StringSlice("buildpacks", buildpacks))
	span.SetAttributes(attribute.String("builder", builder))

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		return nil, "", err
	}

	defer docker.Close() // skipcq: GO-S2307
	defer clearDeploymentTags(ctx, docker, opts.Tag)

	packClient, err := packclient.NewClient(packclient.WithDockerClient(docker), packclient.WithLogger(newPackLogger(streams.Out)))
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		tracing.RecordError(span, err, "failed to create packet client")
		return nil, "", err
	}
	build.BuilderInitFinish()

	build.ImageBuildStart()
	serverInfo, err := docker.Info(ctx)
	if err != nil {
		tracing.RecordError(span, err, "failed to fetch docker server info")
		terminal.Debug("error fetching docker server info:", err)
	} else {
		build.SetBuilderMetaPart2(false, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
	}

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Buildpacks")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	cmdfmt.PrintDone(streams.ErrOut, msg)

	span.AddEvent(msg)

	build.ContextBuildStart()
	excludes, err := readDockerignore(opts.WorkingDir, opts.IgnorefilePath, "")
	if err != nil {
		tracing.RecordError(span, err, "error reading .dockerignore")
		build.ContextBuildFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error reading .dockerignore")
	}
	build.ContextBuildFinish()

	if opts.BuildpacksDockerHost != "" {
		cmdfmt.PrintDone(streams.ErrOut, fmt.Sprintf("buildpacks docker host: %v", opts.BuildpacksDockerHost))
	}
	if len(opts.BuildpacksVolumes) > 0 {
		cmdfmt.PrintDone(streams.ErrOut, fmt.Sprintf("buildpacks volumes: %+v", opts.BuildpacksVolumes))
	}

	buildCtx, buildSpan := tracing.GetTracer().Start(ctx, "build_image",
		trace.WithAttributes(opts.ToSpanAttributes()...),
		trace.WithAttributes(
			attribute.String("type", "buildpack"),
			attribute.Bool("is_remote", dockerFactory.IsRemote()),
		),
	)
	var gid = -1
	var uid = -1
	err = packClient.Build(buildCtx, packclient.BuildOptions{
		AppPath:        opts.WorkingDir,
		Builder:        builder,
		ClearCache:     opts.NoCache,
		Image:          newCacheTag(opts.AppName),
		DockerHost:     opts.BuildpacksDockerHost,
		Buildpacks:     buildpacks,
		Env:            normalizeBuildArgs(opts.BuildArgs),
		UserID:         uid,
		GroupID:        gid,
		TrustBuilder:   returnTrue,
		AdditionalTags: []string{opts.Tag},
		ProjectDescriptor: projectTypes.Descriptor{
			Build: projectTypes.Build{
				Exclude: excludes,
			},
		},
		ContainerConfig: packclient.ContainerConfig{
			Volumes: opts.BuildpacksVolumes,
		},
	})
	build.ImageBuildFinish()
	build.BuildFinish()
	if err != nil {
		if dockerFactory.IsRemote() {
			buildSpan.AddEvent("bad build caused by remote builder failure")
			metrics.SendNoData(ctx, "remote_builder_failure")
		}
		buildSpan.SetAttributes(attribute.Bool("is_remote", dockerFactory.IsRemote()))
		tracing.RecordError(buildSpan, err, "failed to build image")
		buildSpan.End()
		return nil, "", err
	}

	buildSpan.End()

	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	if opts.Publish {
		build.PushStart()
		cmdfmt.PrintBegin(streams.ErrOut, "Pushing image to fly")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			build.PushFinish()
			return nil, "", err
		}
		build.PushFinish()

		cmdfmt.PrintDone(streams.ErrOut, "Pushing image done")
	}

	img, err := findImageWithDocker(ctx, docker, opts.Tag)
	if err != nil {
		return nil, "", err
	}
	if img == nil {
		tracing.RecordError(span, err, "no image found")
		return nil, "", fmt.Errorf("no image found")
	}

	di := DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}

	span.SetAttributes(di.ToSpanAttributes()...)

	return &di, "", nil
}

func normalizeBuildArgs(buildArgs map[string]string) map[string]string {
	out := map[string]string{}

	for k, v := range buildArgs {
		out[k] = v
	}

	return out
}

func newPackLogger(out io.Writer) *packLogger {
	// pack blocks writes to the underlying writer for it's lifetime.
	// we need to use it too, so instead of giving pack stdout/stderr
	// give it a burner writer that we pipe to the target
	packR, packW := io.Pipe()

	go func() {
		io.Copy(out, packR)
		defer packR.Close()
	}()

	return &packLogger{
		w: &fdWrapper{
			Writer: packW,
			src:    out,
		},
		debug: os.Getenv("LOG_LEVEL") == "debug",
	}
}

type packLogger struct {
	w     io.Writer
	debug bool
}

func (l *packLogger) Debug(msg string) {
	if !l.debug {
		return
	}
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Debugf(format string, v ...interface{}) {
	if !l.debug {
		return
	}
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v...)))
}

func (l *packLogger) Info(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Infof(format string, v ...interface{}) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v...)))
}

func (l *packLogger) Warn(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Warnf(format string, v ...interface{}) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v...)))
}

func (l *packLogger) Error(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Errorf(format string, v ...interface{}) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v...)))
}

func (l *packLogger) Writer() io.Writer {
	return l.w
}

func (l *packLogger) IsVerbose() bool {
	return l.debug
}

// fdWrapper creates an io.Writer wrapper that writes to one Writer but reads Fd from another.
// this is used so we can pass the correct Fd through for terminal detection while
// still writing to our piped writer
type fdWrapper struct {
	io.Writer

	src io.Writer
}

type fdWriter interface {
	Fd() uintptr
}

func (w *fdWrapper) Fd() uintptr {
	if fd, ok := w.src.(fdWriter); ok {
		return fd.Fd()
	}
	return ^(uintptr(0))
}
