package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/graphql"
)

type fakeImageAPI struct {
	results []func() (*fly.Image, error)
	calls   int
}

func (f *fakeImageAPI) ResolveImageForApp(context.Context, string, string) (*fly.Image, error) {
	i := min(f.calls, len(f.results)-1)
	f.calls++
	return f.results[i]()
}

func notFound() (*fly.Image, error) {
	gqlErr := &graphql.GraphQLError{Message: `Could not find image "registry.fly.io/app:tag"`}
	gqlErr.Extensions.Code = "NOT_FOUND"
	return nil, fmt.Errorf("failed to run resolve_image: %w", gqlErr)
}

func found() (*fly.Image, error) {
	return &fly.Image{ID: "img", CompressedSize: "1"}, nil
}

var testImageWait = imageWait{timeout: 200 * time.Millisecond, delay: time.Millisecond, max: 5 * time.Millisecond}

func TestResolveImageWithRetryWaitsForPushedImage(t *testing.T) {
	api := &fakeImageAPI{results: []func() (*fly.Image, error){notFound, notFound, found}}
	img, err := resolveImageWithRetry(context.Background(), api, "app", "registry.fly.io/app:tag", testImageWait, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "img", img.ID)
	assert.Equal(t, 3, api.calls)
}

func TestResolveImageWithRetryRetriesNilImage(t *testing.T) {
	api := &fakeImageAPI{results: []func() (*fly.Image, error){
		func() (*fly.Image, error) { return nil, nil },
		found,
	}}
	img, err := resolveImageWithRetry(context.Background(), api, "app", "ref", testImageWait, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "img", img.ID)
	assert.Equal(t, 2, api.calls)
}

func TestResolveImageWithRetryGivesUp(t *testing.T) {
	api := &fakeImageAPI{results: []func() (*fly.Image, error){notFound}}
	start := time.Now()
	_, err := resolveImageWithRetry(context.Background(), api, "app", "ref", testImageWait, io.Discard)
	require.Error(t, err)
	assert.True(t, isImageNotFound(err))
	assert.Greater(t, api.calls, 1)
	assert.Less(t, time.Since(start), time.Second)
}

func TestResolveImageWithRetryReturnsOtherErrorsAtOnce(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeImageAPI{results: []func() (*fly.Image, error){
		func() (*fly.Image, error) { return nil, boom },
	}}
	_, err := resolveImageWithRetry(context.Background(), api, "app", "ref", testImageWait, io.Discard)
	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, api.calls)
}

func TestResolveImageWithRetryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	api := &fakeImageAPI{results: []func() (*fly.Image, error){
		func() (*fly.Image, error) { cancel(); return notFound() },
	}}
	_, err := resolveImageWithRetry(ctx, api, "app", "ref", imageWait{timeout: time.Hour, delay: time.Minute, max: time.Minute}, io.Discard)
	require.ErrorIs(t, err, context.Canceled)
}
