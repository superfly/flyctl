package imgsrc

import (
	"context"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	dockerparser "github.com/novln/docker-parser"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type LocalImageStrategy struct{}

func (s *LocalImageStrategy) Name() string {
	return "Local Image Reference"
}

func imageRefFromOpts(opts ImageOptions) string {
	if opts.ImageRef != "" {
		return opts.ImageRef
	}

	if opts.AppConfig != nil && opts.AppConfig.Build != nil {
		return opts.AppConfig.Build.Image
	}

	return ""
}

func (s *LocalImageStrategy) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}
	ref := imageRefFromOpts(opts)

	if ref == "" {
		terminal.Debug("no image reference found, skipping")
		return nil, nil
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, err
	}

	img, err := findImageWithDocker(docker, ctx, ref)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}

	return di, nil
}

var imageIDPattern = regexp.MustCompile("[a-f0-9]")

func findImageWithDocker(d *dockerclient.Client, ctx context.Context, imageName string) (*types.ImageSummary, error) {
	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	isID := imageIDPattern.MatchString(imageName)

	images, err := d.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	if isID {
		for _, img := range images {
			if len(img.ID) < len(imageName)+7 {
				continue
			}
			if img.ID[7:7+len(imageName)] == imageName {
				terminal.Debug("Found image by id", imageName)
				return &img, nil
			}
		}
	}

	searchTerms := []string{
		imageName,
		imageName + ":" + ref.Tag(),
		ref.Name(),
		ref.ShortName(),
		ref.Remote(),
		ref.Repository(),
	}

	terminal.Debug("Search terms:", searchTerms)

	for _, img := range images {
		for _, tag := range img.RepoTags {
			// skip <none>:<none>
			if strings.HasPrefix(tag, "<none>") {
				continue
			}

			for _, term := range searchTerms {
				if tag == term {
					return &img, nil
				}
			}
		}
	}

	return nil, nil
}
