package docker

import (
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	dockerparser "github.com/novln/docker-parser"
	"github.com/superfly/flyctl/terminal"
)

func (c *DockerClient) FindImage(imageName string) (*types.ImageSummary, error) {
	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	imageIDPattern := regexp.MustCompile("[a-f0-9]")

	isID := imageIDPattern.MatchString(imageName)

	images, err := c.docker.ImageList(c.ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	if isID {
		for _, img := range images {
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
