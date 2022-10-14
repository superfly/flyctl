package imgsrc

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/build/imgsrc/builtins"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
)

type builtinBuilder struct{}

func (ds *builtinBuilder) Name() string {
	return "Builtin"
}

func (ds *builtinBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	build.BuildStart()
	if !dockerFactory.mode.IsAvailable() {
		note := "docker daemon not available, skipping"
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	if opts.BuiltIn == "" {
		note := "fly.toml does not include a builtin config"
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	builtin, err := builtins.GetBuiltin(opts.BuiltIn)
	if err != nil {
		build.BuildFinish()
		return nil, "", err
	}
	// Expand args
	vdockerfile, err := builtin.GetVDockerfile(opts.BuiltInSettings)
	if err != nil {
		build.BuildFinish()
		return nil, "", err
	}

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error connecting to docker")
	}

	build.BuilderInitFinish()
	defer clearDeploymentTags(ctx, docker, opts.Tag)

	build.ContextBuildStart()
	cmdfmt.PrintBegin(streams.ErrOut, "Creating build context")
	archiveOpts := archiveOptions{
		sourcePath: opts.WorkingDir,
		compressed: dockerFactory.IsRemote(),
	}

	excludes, err := readDockerignore(opts.WorkingDir)
	if err != nil {
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error reading .dockerignore")
	}
	archiveOpts.exclusions = excludes

	// copy dockerfile into the archive if it's outside the context dir
	archiveOpts.additions = map[string][]byte{
		"Dockerfile": []byte(vdockerfile),
	}

	r, err := archiveDirectory(archiveOpts)
	if err != nil {
		build.ContextBuildFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error archiving build context")
	}
	build.ContextBuildFinish()
	cmdfmt.PrintDone(streams.ErrOut, "Creating build context done")

	build.ImageBuildStart()
	var imageID string

	serverInfo, err := docker.Info(ctx)
	if err != nil {
		terminal.Debug("error fetching docker server info:", err)
	} else {
		build.SetBuilderMetaPart2(false, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
	}

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Docker")
	msg := fmt.Sprintf("docker host: %s %s %s", serverInfo.ServerVersion, serverInfo.OSType, serverInfo.Architecture)
	cmdfmt.PrintDone(streams.ErrOut, msg)

	buildArgs, err := normalizeBuildArgsForDocker(ctx, opts.BuildArgs)
	if err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		return nil, "", fmt.Errorf("error parsing build args: %w", err)
	}

	imageID, err = runClassicBuild(ctx, streams, docker, r, opts, "", buildArgs)
	if err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "error building")
	}

	build.ImageBuildFinish()
	build.BuildFinish()
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

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, "", errors.Wrap(err, "count not find built image")
	}
	fmt.Println(img)

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, "", nil
}
