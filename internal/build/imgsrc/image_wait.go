package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/superfly/fly-go"
	"github.com/superfly/graphql"
)

// A registry, or a mirror in front of it, can acknowledge a push before the
// image is visible to lookups, so an image pushed moments ago can briefly
// resolve as missing. Lookups of a missing image are retried for this long.
const pushedImageWait = 30 * time.Second

type imageWait struct {
	timeout time.Duration
	delay   time.Duration
	max     time.Duration
}

var defaultImageWait = imageWait{timeout: pushedImageWait, delay: time.Second, max: 5 * time.Second}

// isImageNotFound reports whether err is the API saying the image does not
// exist, as opposed to failing to look it up.
func isImageNotFound(err error) bool {
	var gqlErr *graphql.GraphQLError
	return errors.As(err, &gqlErr) && gqlErr.Extensions.Code == "NOT_FOUND"
}

// resolveImageWithRetry resolves ref through the API, retrying with backoff
// while the image is reported missing and the wait has not run out. Any other
// error is returned at once.
func resolveImageWithRetry(ctx context.Context, api flyClient, appName, ref string, w imageWait, out io.Writer) (*fly.Image, error) {
	return waitForImage(ctx, ref, w, out, func() (*fly.Image, error, bool) {
		img, err := api.ResolveImageForApp(ctx, appName, ref)

		return img, err, (err == nil && img == nil) || isImageNotFound(err)
	})
}

func inspectPushedTag(ctx context.Context, docker *dockerclient.Client, tag, auth string, w imageWait, out io.Writer) (registry.DistributionInspect, error) {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	return waitForImage(ctx, tag, w, out, func() (registry.DistributionInspect, error, bool) {
		current, err := docker.DistributionInspect(ctx, tag, auth)
		retry := errdefs.IsNotFound(err) || errdefs.IsUnavailable(err) || dockerclient.IsErrConnectionFailed(err)

		return current, err, retry
	})
}

func waitForImage[T any](ctx context.Context, ref string, w imageWait, out io.Writer, lookup func() (T, error, bool)) (T, error) {
	deadline := time.Now().Add(w.timeout)
	delay := w.delay
	announced := false
	for {
		img, err, retry := lookup()
		if !retry || time.Now().Add(delay).After(deadline) {
			return img, err
		}
		if !announced {
			fmt.Fprintf(out, "Registry lookup for %s has not succeeded yet, retrying for up to %s...\n", ref, w.timeout)
			announced = true
		}
		select {
		case <-ctx.Done():
			var zero T

			return zero, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, w.max)
	}
}
