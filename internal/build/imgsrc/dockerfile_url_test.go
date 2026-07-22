package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func setTempDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
}

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
	}, nil, NewDockerfileMaterializer())

	require.NoError(t, err)
	assert.Same(t, expected, image)
	assert.Zero(t, requests.Load())
	assert.NotImplements(t, (*dockerfileConsumer)(nil), &buildpacksBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &dockerfileBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &BuildkitBuilder{})
	assert.Implements(t, (*dockerfileConsumer)(nil), &DepotBuilder{})
}

func TestImageOptionsRedactsMalformedDockerfileURL(t *testing.T) {
	const dockerfileURL = "https://" + "user:pass@" + "example.com/%zz?token=secret#fragment"
	var got string
	for _, attr := range (ImageOptions{DockerfilePath: dockerfileURL}).ToSpanAttributes() {
		if string(attr.Key) == "imageoptions.dockerfile_path" {
			got = attr.Value.AsString()
		}
	}

	assert.Equal(t, "invalid URL", got)
}

func TestRunImageBuilderMaterializesDockerfileURLAndCleansUp(t *testing.T) {
	tempRoot := t.TempDir()
	setTempDir(t, tempRoot)

	const content = "FROM alpine:latest\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	t.Cleanup(server.Close)

	var materializedPath string
	materializer := NewDockerfileMaterializer()
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
	}, nil, materializer)

	require.NoError(t, err)
	require.NoError(t, materializer.Close())
	_, err = os.Stat(filepath.Dir(materializedPath))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunImageBuilderCleansUpAfterBuilderError(t *testing.T) {
	tempRoot := t.TempDir()
	setTempDir(t, tempRoot)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "FROM alpine:latest\n")
	}))
	t.Cleanup(server.Close)

	expectedErr := errors.New("builder failed")
	var materializedDir string
	materializer := NewDockerfileMaterializer()
	strategy := &dockerfileBuilderFunc{imageBuilderFunc: func(_ context.Context, _ *dockerClientFactory, _ *iostreams.IOStreams, opts ImageOptions, _ *build) (*DeploymentImage, string, error) {
		materializedDir = filepath.Dir(opts.DockerfilePath)

		return nil, "", expectedErr
	}}

	_, _, err := runImageBuilder(context.Background(), strategy, nil, nil, ImageOptions{
		DockerfilePath: server.URL + "/Dockerfile",
	}, nil, materializer)

	assert.ErrorIs(t, err, expectedErr)
	require.NoError(t, materializer.Close())
	_, err = os.Stat(materializedDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunImageBuilderReusesDockerfileAcrossFailover(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, "FROM alpine:latest\n")
	}))
	t.Cleanup(server.Close)

	materializer := NewDockerfileMaterializer()
	var paths []string
	expectedErr := errors.New("builder connection failed")
	strategy := &dockerfileBuilderFunc{imageBuilderFunc: func(_ context.Context, _ *dockerClientFactory, _ *iostreams.IOStreams, opts ImageOptions, _ *build) (*DeploymentImage, string, error) {
		paths = append(paths, opts.DockerfilePath)
		if len(paths) == 1 {
			return nil, "", expectedErr
		}

		return &DeploymentImage{Tag: "dockerfile-image"}, "", nil
	}}
	opts := ImageOptions{DockerfilePath: server.URL + "/Dockerfile"}

	_, _, err := runImageBuilder(context.Background(), strategy, nil, nil, opts, nil, materializer)
	require.ErrorIs(t, err, expectedErr)
	_, _, err = runImageBuilder(context.Background(), strategy, nil, nil, opts, nil, materializer)

	require.NoError(t, err)
	require.NoError(t, materializer.Close())
	assert.Equal(t, int32(1), requests.Load())
	require.Len(t, paths, 2)
	assert.Equal(t, paths[0], paths[1])
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

	require.NoError(t, cleanup())
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadDockerfileAcceptsUppercaseScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "FROM alpine:latest\n")
	}))
	t.Cleanup(server.Close)
	dockerfileURL := strings.Replace(server.URL, "http://", "HTTP://", 1) + "/Dockerfile"

	path, cleanup, err := downloadDockerfile(context.Background(), dockerfileURL, time.Second, 1024)

	require.NoError(t, err)
	require.NoError(t, cleanup())
	assert.NotEmpty(t, path)
}

func TestDockerfileMaterializerReportsCleanupFailure(t *testing.T) {
	expectedErr := errors.New("remove failed")
	materializer := &DockerfileMaterializer{
		cleanup: func() error { return expectedErr },
	}

	err := materializer.Close()

	assert.ErrorIs(t, err, expectedErr)
	assert.ErrorIs(t, materializer.Close(), expectedErr)
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
	setTempDir(t, tempDir)

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
	dockerfileURL := "https://" + "user:password@" + "example.com/Dockerfile?token=secret#fragment"

	path, cleanup, err := downloadDockerfile(ctx, dockerfileURL, time.Second, 1024)

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

func TestDownloadDockerfileRedactsMalformedRedirectURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://"+"user:pass@"+"example.com/%zz?token=secret#fragment")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, 1024)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorContains(t, err, "failed to parse redirect Location header")
	assert.NotContains(t, err.Error(), "user")
	assert.NotContains(t, err.Error(), "pass")
	assert.NotContains(t, err.Error(), "token")
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "fragment")
}

func TestDownloadDockerfileRedactsRelativeRedirectURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/Dockerfile?token=secret#fragment")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	path, cleanup, err := downloadDockerfile(context.Background(), server.URL+"/Dockerfile", time.Second, 1024)

	assert.Empty(t, path)
	assert.Nil(t, cleanup)
	assert.ErrorContains(t, err, "/Dockerfile")
	assert.NotContains(t, err.Error(), "token")
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "fragment")
}
