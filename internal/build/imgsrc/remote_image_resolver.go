package imgsrc

import (
	"context"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type remoteImageResolver struct {
	flyApi *api.Client
}

func (s *remoteImageResolver) Name() string {
	return "Remote Image Reference"
}

func (s *remoteImageResolver) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	ref := imageRefFromOpts(opts)
	if ref == "" {
		terminal.Debug("no image reference found, skipping")
		return nil, nil
	}

	img, err := s.flyApi.ResolveImageForApp(opts.AppName, ref)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: int64(img.CompressedSize),
	}

	return di, nil
}
