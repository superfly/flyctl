package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

type imageBuilderFunc func(context.Context, *dockerClientFactory, *iostreams.IOStreams, ImageOptions, *build) (*DeploymentImage, string, error)

func (f imageBuilderFunc) Name() string { return "test" }

func (f imageBuilderFunc) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	return f(ctx, dockerFactory, streams, opts, build)
}

type dockerfileBuilderFunc struct {
	imageBuilderFunc
}

func (*dockerfileBuilderFunc) usesDockerfile() {}

func TestRunImageBuilderDoesNotFetchUnusedDockerfileURL(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)

	expected := &DeploymentImage{Tag: "buildpack-image"}
	strategy := imageBuilderFunc(func(_ context.Context, _ *dockerClientFactory, _ *iostreams.IOStreams, opts ImageOptions, _ *build) (*DeploymentImage, string, error) {
		assert.Equal(t, server.URL+"/Dockerfile", opts.DockerfilePath)

		return expected, "", nil
	})

	image, _, err := runImageBuilder(context.Background(), strategy, nil, nil, ImageOptions{
		Builder:        "paketobuildpacks/builder-jammy-base",
		DockerfilePath: server.URL + "/Dockerfile",
	}, nil)

	require.NoError(t, err)
	assert.Same(t, expected, image)
	assert.Zero(t, requests.Load())
	assert.NotImplements(t, (*dockerfileConsumer)(nil), &buildpacksBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &dockerfileBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &BuildkitBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &DepotBuilder{})
}

func TestRunImageBuilderMaterializesDockerfileURLAndCleansUp(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	const content = "FROM alpine:latest\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	t.Cleanup(server.Close)

	var materializedPath string
	strategy := &dockerfileBuilderFunc{imageBuilderFunc: func(_ context.Context, _ *dockerClientFactory, _ *iostreams.IOStreams, opts ImageOptions, _ *build) (*DeploymentImage, string, error) {
		materializedPath = opts.DockerfilePath
		assert.Equal(t, filepath.Join(filepath.Dir(materializedPath), "Dockerfile"), materializedPath)
		assert.Equal(t, tempRoot, filepath.Dir(filepath.Dir(materializedPath)))

		entries, err := os.ReadDir(filepath.Dir(materializedPath))
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "Dockerfile", entries[0].Name())

		got, err := os.ReadFile(materializedPath)
		require.NoError(t, err)
		assert.Equal(t, content, string(got))

		return &DeploymentImage{Tag: "dockerfile-image"}, "", nil
	}}

	_, _, err := runImageBuilder(context.Background(), strategy, nil, nil, ImageOptions{
		DockerfilePath: server.URL + "/Dockerfile",
	}, nil)

	require.NoError(t, err)
	_, err = os.Stat(filepath.Dir(materializedPath))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunImageBuilderCleansUpAfterBuilderError(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "FROM alpine:latest\n")
	}))
	t.Cleanup(server.Close)

	expectedErr := errors.New("builder failed")
	var materializedDir string
	strategy := &dockerfileBuilderFunc{imageBuilderFunc: func(_ context.Context, _ *dockerClientFactory, _ *iostreams.IOStreams, opts ImageOptions, _ *build) (*DeploymentImage, string, error) {
		materializedDir = filepath.Dir(opts.DockerfilePath)
		return nil, "", expectedErr
	}}

	_, _, err := runImageBuilder(context.Background(), strategy, nil, nil, ImageOptions{
		DockerfilePath: server.URL + "/Dockerfile",
	}, nil)

	assert.ErrorIs(t, err, expectedErr)
	_, err = os.Stat(materializedDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadDockerfile(t *testing.T) {
	const content = "FROM alpine:latest\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, int64(len(content)))
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))

	cleanup()
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadDockerfileRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, 1024)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorContains(t, err, "404 Not Found")
}

func TestDownloadDockerfileRejectsOversizedStreamAndCleansUp(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "123456789")
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, 8)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorContains(t, err, "response exceeds 8-byte limit")
	entries, readErr := os.ReadDir(tempDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestDownloadDockerfileRejectsDeclaredOversize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "9")
		fmt.Fprint(w, "123456789")
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, 8)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorContains(t, err, "response exceeds 8-byte limit")
}

func TestDownloadDockerfileTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", 50*time.Millisecond, 1024)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDownloadDockerfileRedactsURLFromRequestError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	path, cleanup, err := downloadDockerfile(ctx, "https://user:password@example.com/Dockerfile?token=secret#fragment", time.Second, 1024)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "https://example.com/Dockerfile")
	assert.NotContains(t, err.Error(), "user")
	assert.NotContains(t, err.Error(), "password")
	assert.NotContains(t, err.Error(), "token")
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "fragment")
}

func TestRedactDockerfileURL(t *testing.T) {
	assert.Equal(t, "Dockerfile.custom", redactDockerfileURL("Dockerfile.custom"))
	assert.Equal(t,
		"https://example.com/path/Dockerfile",
		redactDockerfileURL("https://user:password@example.com/path/Dockerfile?token=secret#fragment"),
	)
}
