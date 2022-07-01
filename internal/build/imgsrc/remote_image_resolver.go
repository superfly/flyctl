package imgsrc

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"
)

type remoteImageResolver struct {
	flyApi *api.Client
}

func (*remoteImageResolver) Name() string {
	return "Remote Image Reference"
}

func (s *remoteImageResolver) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions) (*DeploymentImage, error) {

	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' remotely...\n", opts.ImageRef)

	img, err := s.flyApi.ResolveImageForApp(ctx, opts.AppName, opts.ImageRef)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}

	fmt.Fprintf(streams.ErrOut, "image found: %s\n", img.ID)

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  img.Ref,
		Size: int64(img.CompressedSize),
	}

	return di, nil
}
