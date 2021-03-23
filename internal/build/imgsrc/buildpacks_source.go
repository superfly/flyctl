package imgsrc

import (
	"context"
	"fmt"
	"io"

	"github.com/buildpacks/pack"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type buildpacksStrategy struct{}

func (ds *buildpacksStrategy) Name() string {
	return "Buildpacks"
}

func (s *buildpacksStrategy) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
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

	packClient, err := pack.NewClient(pack.WithDockerClient(docker), pack.WithLogger(&packLogger{w: streams.Out, debug: false}))
	if err != nil {
		return nil, err
	}

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Buildpacks")

	err = packClient.Build(ctx, pack.BuildOptions{
		AppPath:      opts.WorkingDir,
		Builder:      builder,
		Image:        opts.Tag,
		Buildpacks:   buildpacks,
		Env:          normalizeBuildArgs(opts.AppConfig, opts.ExtraBuildArgs),
		TrustBuilder: true,
		// Publish:      opts.Publish,
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
	fmt.Fprintf(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v)))
}

func (l *packLogger) Info(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Infof(format string, v ...interface{}) {
	fmt.Fprintf(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v)))
}

func (l *packLogger) Warn(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Warnf(format string, v ...interface{}) {
	fmt.Fprintf(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v)))
}

func (l *packLogger) Error(msg string) {
	fmt.Fprint(l.w, cmdfmt.AppendMissingLineFeed(msg))
}

func (l *packLogger) Errorf(format string, v ...interface{}) {
	fmt.Fprintf(l.w, cmdfmt.AppendMissingLineFeed(fmt.Sprintf(format, v)))
}

func (l *packLogger) Writer() io.Writer {
	return l.w
}

func (l *packLogger) IsVerbose() bool {
	return l.debug
}
