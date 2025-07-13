package imgsrc

import (
	"context"
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/superfly/flyctl/iostreams"
	"net/http"
)

var _ imageResolver = (*FlapsResolver)(nil)

func NewFlapsResolver(addr string) *FlapsResolver {
	return &FlapsResolver{addr: addr}
}

func (r *FlapsResolver) Name() string { return "Flaps" }

type FlapsResolver struct{ addr string }

func (r *FlapsResolver) Run(
	ctx context.Context,
	_ *dockerClientFactory,
	streams *iostreams.IOStreams,
	opts RefOptions,
	build *build,
) (*DeploymentImage, string, error) {
	_, _ = fmt.Fprintf(streams.ErrOut, "Searching for image '%s' in registry...\n", opts.ImageRef)

	build.BuildStart()
	defer build.BuildFinish()

	ref, err := name.ParseReference(opts.ImageRef)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	img, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithTransport(&http.Transport{}),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get image: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get manifest: %w", err)
	}

	imgDigest, err := img.Digest()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get image digest: %w", err)
	}

	var size int64
	size += manifest.Config.Size
	for _, layer := range manifest.Layers {
		size += layer.Size
	}

	var tag string
	if refTag, ok := ref.(name.Tag); ok {
		tag = refTag.TagStr()
	}

	imageId, err := img.ConfigName()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get image config name: %w", err)
	}

	deploymentImage := &DeploymentImage{
		ID:     imageId.String(),
		Tag:    tag,
		Digest: imgDigest.String(),
		Size:   size,
	}

	fmt.Fprintf(streams.ErrOut, "image found: %s\n", deploymentImage.ID)
	return deploymentImage, imgDigest.String(), nil
}
