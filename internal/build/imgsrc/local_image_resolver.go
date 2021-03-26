package imgsrc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	dockerparser "github.com/novln/docker-parser"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type localImageResolver struct{}

func (s *localImageResolver) Name() string {
	return "Local Image Reference"
}

func imageRefFromOpts(opts RefOptions) string {
	if opts.ImageRef != "" {
		return opts.ImageRef
	}

	if opts.AppConfig != nil && opts.AppConfig.Build != nil {
		return opts.AppConfig.Build.Image
	}

	return ""
}

func (s *localImageResolver) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsLocal() {
		terminal.Debug("local docker daemon not available, skipping")
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

	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' locally...\n", ref)

	img, err := findImageWithDocker(docker, ctx, ref)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}

	fmt.Fprintf(streams.ErrOut, "image found: %s\n", img.ID)

	if opts.Publish {
		err = docker.ImageTag(ctx, img.ID, opts.Tag)
		if err != nil {
			return nil, errors.Wrap(err, "error tagging image")
		}

		defer clearDeploymentTags(ctx, docker, opts.Tag)

		cmdfmt.PrintBegin(streams.ErrOut, "Pushing image to fly")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			return nil, err
		}

		cmdfmt.PrintDone(streams.ErrOut, "Pushing image done")
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
