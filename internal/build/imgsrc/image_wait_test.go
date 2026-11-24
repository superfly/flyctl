package imgsrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/opencontainers/go-digest"
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
	var out bytes.Buffer
	img, err := resolveImageWithRetry(context.Background(), api, "app", "registry.fly.io/app:tag", testImageWait, &out)
	assert.Equal(t, "Image registry.fly.io/app:tag is not visible in the registry yet, retrying for up to 200ms...\n", out.String())
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

func TestInspectPushedTagRetriesTransientErrors(t *testing.T) {
	ack := digest.FromString("manifest")
	for _, tc := range []struct {
		name        string
		firstStatus int
		wantCalls   int32
	}{
		{"not visible", http.StatusNotFound, 2},
		{"unavailable", http.StatusServiceUnavailable, 2},
		{"connection closed", 0, 2},
		{"forbidden", http.StatusForbidden, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					if tc.firstStatus == 0 {
						panic(http.ErrAbortHandler)
					}
					http.Error(w, "inspection failed", tc.firstStatus)

					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"Descriptor":{"digest":%q}}`, ack)
			}))
			defer daemon.Close()
			docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://"+daemon.Listener.Addr().String()), dockerclient.WithVersion("1.41"))
			require.NoError(t, err)
			defer docker.Close()

			var out bytes.Buffer
			manifest, err := inspectPushedTag(context.Background(), docker, "registry.fly.io/app:tag", "", testImageWait, &out)
			require.Equal(t, tc.wantCalls, calls.Load())
			if tc.wantCalls == 1 {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, ack, manifest.Descriptor.Digest)
			require.Equal(t, "Registry lookup for registry.fly.io/app:tag has not succeeded yet, retrying for up to 200ms...\n", out.String())
		})
	}
}

func TestInspectPushedTagBoundsInFlightRequest(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer daemon.Close()
	docker, err := dockerclient.NewClientWithOpts(dockerclient.WithHost("tcp://"+daemon.Listener.Addr().String()), dockerclient.WithVersion("1.41"))
	require.NoError(t, err)
	defer docker.Close()

	_, err = inspectPushedTag(context.Background(), docker, "registry.fly.io/app:tag", "", testImageWait, io.Discard)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
