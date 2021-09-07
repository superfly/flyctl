package imgsrc

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/buildpacks/pack"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type buildpacksBuilder struct{}

func (ds *buildpacksBuilder) Name() string {
	return "Buildpacks"
}

func (s *buildpacksBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}

	if !opts.AppConfig.HasBuilder() {
		terminal.Debug("no buildpack builder configured, skipping")
		return nil, nil
	}

	builder := opts.AppConfig.Build.Builder
	buildpacks := opts.AppConfig.Build.Buildpacks

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, err
	}

	defer clearDeploymentTags(ctx, docker, opts.Tag)

	packClient, err := pack.NewClient(pack.WithDockerClient(docker), pack.WithLogger(newPackLogger(streams.Out)))
	if err != nil {
		return nil, err
	}

	serverInfo, err := docker.Info(ctx)
	if err != nil {
		terminal.Debug("error fetching docker server info:", err)
	}

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Buildpacks")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	cmdfmt.PrintDone(streams.ErrOut, msg)

	err = packClient.Build(ctx, pack.BuildOptions{
		AppPath:        opts.WorkingDir,
		Builder:        builder,
		Image:          newCacheTag(opts.AppName),
		Buildpacks:     buildpacks,
		Env:            normalizeBuildArgs(opts.AppConfig, opts.ExtraBuildArgs),
		TrustBuilder:   true,
		AdditionalTags: []string{opts.Tag},
	})

	if err != nil {
		return nil, err
	}

	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	if opts.Publish {
		cmdfmt.PrintBegin(streams.ErrOut, "Pushing image to fly")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			return nil, err
		}

		cmdfmt.PrintDone(streams.ErrOut, "Pushing image done")
	}

	img, err := findImageWithDocker(docker, ctx, opts.Tag)
	if err != nil {
		return nil, err
	}

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, nil
}

func normalizeBuildArgs(appConfig *flyctl.AppConfig, extra map[string]string) map[string]string {
	var out = map[string]string{}

	if appConfig.Build != nil {
		for k, v := range appConfig.Build.Args {
			out[k] = v
		}
	}

	for k, v := range extra {
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
