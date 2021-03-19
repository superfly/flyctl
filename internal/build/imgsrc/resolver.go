package imgsrc

import (
	"context"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type ImageOptions struct {
	AppName        string
	WorkingDir     string
	DockerfilePath string
	ImageRef       string
	AppConfig      *flyctl.AppConfig
	ExtraBuildArgs map[string]string
	ImageLabel     string
	Publish        bool
	Tag            string
}

type Resolver struct {
	dockerFactory *dockerClientFactory
	apiClient     *api.Client
}

func (r *Resolver) Resolve(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions) (img *DeploymentImage, err error) {
	if opts.Tag == "" {
		opts.Tag = docker.NewDeploymentTag(opts.AppName, opts.ImageLabel)
	}

	strategies := []resolverStrategy{
		&LocalImageStrategy{},
		&RemoteImageStrategy{flyApi: r.apiClient},
		&dockerfileStrategy{},
		&buildpacksStrategy{},
	}

	for _, s := range strategies {
		terminal.Debugf("Trying '%s' strategy\n", s.Name())
		img, err = s.Run(ctx, r.dockerFactory, streams, opts)
		terminal.Debugf("result image:%+v error:%v\n", img, err)
		if err != nil {
			return nil, err
		}
		if img != nil {
			return img, nil
		}
	}

	return nil, nil
}

func NewResolver(daemonType DockerDaemonType, apiClient *api.Client, appName string) *Resolver {
	return &Resolver{
		dockerFactory: NewDockerClientFactory(daemonType, apiClient, appName),
		apiClient:     apiClient,
	}
}

type resolverStrategy interface {
	Name() string
	Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error)
}
