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
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type localImageResolver struct{}

func (*localImageResolver) Name() string {
	return "Local Image Reference"
}

func (*localImageResolver) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions) (*DeploymentImage, error) {
	if !dockerFactory.IsLocal() {
		terminal.Debug("local docker daemon not available, skipping")
		return nil, nil
	}

	if opts.Tag == "" {
		opts.Tag = NewDeploymentTag(opts.AppName, opts.ImageLabel)
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' locally...\n", opts.ImageRef)

	img, err := findImageWithDocker(ctx, docker, opts.ImageRef)
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

func findImageWithDocker(ctx context.Context, d *dockerclient.Client, imageName string) (*types.ImageSummary, error) {
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
