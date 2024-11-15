package imgsrc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	dockerparser "github.com/novln/docker-parser"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
)

type localImageResolver struct{}

func (*localImageResolver) Name() string {
	return "Local Image Reference"
}

func (*localImageResolver) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "local_image_resolver")
	defer span.End()

	build.BuildStart()
	if !dockerFactory.IsLocal() {
		note := "local docker daemon not available, skipping"
		terminal.Debug(note)
		span.AddEvent(note)
		build.BuildFinish()
		return nil, note, nil
	}

	if opts.Tag == "" {
		opts.Tag = NewDeploymentTag(opts.AppName, opts.ImageLabel)
	}

	span.SetAttributes(opts.ToSpanAttributes()...)

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	build.BuilderInitFinish()
	if err != nil {
		build.BuildFinish()
		return nil, "", err
	}
	defer docker.Close() // skipcq: GO-S2307

	serverInfo, err := docker.Info(ctx)
	if err != nil {
		span.AddEvent(fmt.Sprintf("error fetching docker server info:%s", err.Error()))
		terminal.Debug("error fetching docker server info:", err)
	} else {
		buildkitEnabled, err := buildkitEnabled(docker)
		terminal.Debugf("buildkitEnabled %v", buildkitEnabled)
		span.SetAttributes(attribute.Bool("docker.buildkit_enabled", buildkitEnabled))
		if err == nil {
			build.SetBuilderMetaPart2(buildkitEnabled, serverInfo.ServerVersion, fmt.Sprintf("%s/%s/%s", serverInfo.OSType, serverInfo.Architecture, serverInfo.OSVersion))
		}
	}

	fmt.Fprintf(streams.ErrOut, "Searching for image '%s' locally...\n", opts.ImageRef)

	img, err := findImageWithDocker(ctx, docker, opts.ImageRef)
	if err != nil {
		build.BuildFinish()
		tracing.RecordError(span, err, "failed to find image with docker")
		return nil, "", err
	}
	if img == nil {
		build.BuildFinish()
		span.AddEvent("no image found and no error occurred")
		return nil, "no image found and no error occurred", nil
	}

	build.BuildFinish()
	fmt.Fprintf(streams.ErrOut, "image found: %s\n", img.ID)

	span.SetAttributes(attribute.String("image.id", img.ID))

	if opts.Publish {
		build.PushStart()
		err = docker.ImageTag(ctx, img.ID, opts.Tag)
		if err != nil {
			build.PushFinish()
			tracing.RecordError(span, err, "failed to tag image")
			return nil, "", errors.Wrap(err, "error tagging image")
		}

		defer clearDeploymentTags(ctx, docker, opts.Tag)

		cmdfmt.PrintBegin(streams.ErrOut, "Pushing image to fly")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			build.PushFinish()
			return nil, "", err
		}

		cmdfmt.PrintDone(streams.ErrOut, "Pushing image done")
	}

	di := &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}

	span.SetAttributes(di.ToSpanAttributes()...)

	return di, "", nil
}

var imageIDPattern = regexp.MustCompile("[a-f0-9]")

func findImageWithDocker(ctx context.Context, d *dockerclient.Client, imageName string) (*image.Summary, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "find_image_with_docker")
	defer span.End()

	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		tracing.RecordError(span, err, "failed to parse image")
		return nil, err
	}

	span.SetAttributes(attribute.String("ref", ref.Name()))

	isID := imageIDPattern.MatchString(imageName)

	images, err := d.ImageList(ctx, image.ListOptions{})
	if err != nil {
		tracing.RecordError(span, err, "failed to list images")
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

	span.SetAttributes(attribute.StringSlice("search_terms", searchTerms))

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
