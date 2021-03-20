package imgsrc

import (
	"context"
	"fmt"

	"github.com/buildpacks/pack"
	"github.com/superfly/flyctl/flyctl"
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

	packClient, err := pack.NewClient(pack.WithDockerClient(docker))
	if err != nil {
		return nil, err
	}

	err = packClient.Build(ctx, pack.BuildOptions{
		AppPath:      opts.WorkingDir,
		Builder:      builder,
		Image:        opts.Tag,
		Buildpacks:   buildpacks,
		Env:          normalizeBuildArgs(opts.AppConfig, opts.ExtraBuildArgs),
		TrustBuilder: true,
		Publish:      opts.Publish,
	})

	if err != nil {
		return nil, err
	}

	img, err := findImageWithDocker(docker, ctx, opts.Tag)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%+v\n", img)

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, nil
}

func normalizeBuildArgs(appConfig *flyctl.AppConfig, extra map[string]string) map[string]string {
	fmt.Println(appConfig, extra)
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
