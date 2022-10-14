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

func (s *remoteImageResolver) Run(ctx context.Context, _ *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions, build *build) (*DeploymentImage, string, error) {
	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' remotely...\n", opts.ImageRef)

	build.BuildStart()
	img, err := s.flyApi.ResolveImageForApp(ctx, opts.AppName, opts.ImageRef)
	build.BuildFinish()
	if err != nil {
		return nil, "", err
	}
	if img == nil {
		return nil, "no image found and no error occurred", nil
	}

	fmt.Fprintf(streams.ErrOut, "image found: %s\n", img.ID)

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  img.Ref,
		Size: int64(img.CompressedSize),
	}

	return di, "", nil
}
