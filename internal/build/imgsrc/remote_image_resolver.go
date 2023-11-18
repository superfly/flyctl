package imgsrc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

type remoteImageResolver struct {
	flyApi *api.Client
}

func (*remoteImageResolver) Name() string {
	return "Remote Image Reference"
}

func (s *remoteImageResolver) Run(ctx context.Context, _ *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "resolve_image_remotely")
	defer span.End()

	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' remotely...\n", opts.ImageRef)

	build.BuildStart()
	img, err := s.flyApi.ResolveImageForApp(ctx, opts.AppName, opts.ImageRef)
	build.BuildFinish()
	if err != nil {
		tracing.RecordError(span, err, "failed to resolve image")
		return nil, "", err
	}
	if img == nil {
		span.AddEvent("no image found and no error occurred")
		return nil, "no image found and no error occurred", nil
	}

	fmt.Fprintf(streams.ErrOut, "image found: %s\n", img.ID)

	size, err := strconv.ParseUint(img.CompressedSize, 10, 64)
	if err != nil {
		tracing.RecordError(span, err, "failed to parse size")
		return nil, "", err
	}

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  img.Ref,
		Size: int64(size),
	}

	span.SetAttributes(di.ToSpanAttributes()...)

	return di, "", nil
}
