package imgsrc

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/buildpacks/pack"
	projectTypes "github.com/buildpacks/pack/pkg/project/types"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type buildpacksBuilder struct{}

func (*buildpacksBuilder) Name() string {
	return "Buildpacks"
}

func (*buildpacksBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	build.BuildStart()
	if !dockerFactory.mode.IsAvailable() {
		note := "docker daemon not available, skipping"
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	if opts.Builder == "" {
		note := "no buildpack builder configured, skipping"
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	builder := opts.Builder
	buildpacks := opts.Buildpacks

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		return nil, "", err
	}

	defer clearDeploymentTags(ctx, docker, opts.Tag)

	packClient, err := pack.NewClient(pack.WithDockerClient(docker), pack.WithLogger(newPackLogger(streams.Out)))
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		return nil, "", err
	}
	build.BuilderInitFinish()

	build.ImageBuildStart()
	serverInfo, err := docker.Info(ctx)
	if err != nil {
		terminal.Debug("error fetching docker server info:", err)
	} else {
		build.SetBuilderMetaPart2(false, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
	}

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Buildpacks")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	cmdfmt.PrintDone(streams.ErrOut, msg)

	build.ContextBuildStart()
	excludes, err := readDockerignore(opts.WorkingDir, opts.IgnorefilePath)
	if err != nil {
		build.ContextBuildFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error reading .dockerignore")
	}
	build.ContextBuildFinish()

	err = packClient.Build(ctx, pack.BuildOptions{
		AppPath:        opts.WorkingDir,
		Builder:        builder,
		ClearCache:     opts.NoCache,
		Image:          newCacheTag(opts.AppName),
		Buildpacks:     buildpacks,
		Env:            normalizeBuildArgs(opts.BuildArgs),
		TrustBuilder:   true,
		AdditionalTags: []string{opts.Tag},
		ProjectDescriptor: projectTypes.Descriptor{
			Build: projectTypes.Build{
				Exclude: excludes,
			},
		},
	})
	build.ImageBuildFinish()
	build.BuildFinish()

	if err != nil {
		return nil, "", err
	}

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

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, "", nil
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
