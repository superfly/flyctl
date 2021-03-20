package imgsrc

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/build/imgsrc/builtins"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
)

type builtinBuilder struct{}

func (ds *builtinBuilder) Name() string {
	return "Builtin"
}

func (ds *builtinBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}

	if !opts.AppConfig.HasBuiltin() {
		terminal.Debug("fly.toml does not include a builtin config")
		return nil, nil
	}

	builtin, err := builtins.GetBuiltin(opts.AppConfig.Build.Builtin)
	if err != nil {
		return nil, err
	}
	// Expand args
	vdockerfile, err := builtin.GetVDockerfile(opts.AppConfig.Build.Settings)
	if err != nil {
		return nil, err
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to docker")
	}

	defer clearDeploymentTags(ctx, docker, opts.Tag)

	fmt.Println("building archive")
	archiveOpts := archiveOptions{
		sourcePath: opts.WorkingDir,
		compressed: dockerFactory.mode.IsRemote(),
	}

	excludes, err := readDockerignore(opts.WorkingDir)
	if err != nil {
		return nil, errors.Wrap(err, "error reading .dockerignore")
	}
	archiveOpts.exclusions = excludes

	// copy dockerfile into the archive if it's outside the context dir
	archiveOpts.additions = map[string][]byte{
		"Dockerfile": []byte(vdockerfile),
	}

	r, err := archiveDirectory(archiveOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error archiving build context")
	}
	fmt.Println("building archive done")

	var imageID string

	fmt.Println("building image")

	buildArgs := normalizeBuildArgsForDocker(opts.AppConfig, opts.ExtraBuildArgs)
	imageID, err = runClassicBuild(ctx, streams, docker, r, opts, buildArgs)
	if err != nil {
		return nil, errors.Wrap(err, "error building")
	}

	fmt.Println("building image done")

	if opts.Publish {
		fmt.Println("pushing image")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			return nil, err
		}

		fmt.Println("pushing image done ")
	}

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, errors.Wrap(err, "count not find built image")
	}
	fmt.Println(img)

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, nil

}
