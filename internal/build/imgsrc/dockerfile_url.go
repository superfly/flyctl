package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superfly/flyctl/internal/dockerfileurl"
)

const (
	dockerfileDownloadTimeout = 30 * time.Second
	maxDockerfileSizeBytes    = 10 << 20
)

// DockerfileMaterializer lazily resolves one remote Dockerfile and keeps that
// exact snapshot available across builder connection failover.
type DockerfileMaterializer struct {
	attempted bool
	closed    bool
	source    string
	path      string
	cleanup   func() error
	err       error
}

// NewDockerfileMaterializer returns an empty lazy materializer.
func NewDockerfileMaterializer() *DockerfileMaterializer {
	return new(DockerfileMaterializer)
}

func redactDockerfileRequestError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}

	redacted := *urlErr
	redacted.URL = dockerfileurl.ForRequestError(urlErr.URL)
	if strings.HasPrefix(urlErr.Err.Error(), "failed to parse Location header ") {
		redacted.Err = errors.New("failed to parse redirect Location header")
	}

	return &redacted
}

func (m *DockerfileMaterializer) materialize(ctx context.Context, path string) (string, error) {
	if !dockerfileurl.IsURL(path) {
		return path, nil
	}
	if m.closed {
		return "", errors.New("Dockerfile materializer is closed")
	}
	if m.attempted {
		if path != m.source {
			return "", errors.New("Dockerfile materializer cannot resolve multiple sources")
		}

		return m.path, m.err
	}

	m.attempted = true
	m.source = path
	m.path, m.cleanup, m.err = downloadDockerfile(ctx, path, dockerfileDownloadTimeout, maxDockerfileSizeBytes)

	return m.path, m.err
}

func (m *DockerfileMaterializer) Close() error {
	if m.closed {
		return nil
	}
	if m.cleanup == nil {
		m.closed = true

		return nil
	}

	err := m.cleanup()
	if err != nil {
		return fmt.Errorf("removing temporary Dockerfile directory: %w", err)
	}
	m.cleanup = nil
	m.closed = true

	return nil
}

func downloadDockerfile(ctx context.Context, dockerfileURL string, timeout time.Duration, maxBytes int64) (path string, cleanup func() error, err error) {
	downloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, dockerfileURL, http.NoBody)
	if err != nil {
		return "", nil, fmt.Errorf("creating Dockerfile request: %w", redactDockerfileRequestError(err))
	}
	req.URL.Scheme = strings.ToLower(req.URL.Scheme)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("downloading Dockerfile: %w", redactDockerfileRequestError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, fmt.Errorf("downloading Dockerfile: unexpected HTTP status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if resp.ContentLength > maxBytes {
		return "", nil, fmt.Errorf("downloading Dockerfile: response exceeds %d-byte limit", maxBytes)
	}

	tempDir, err := os.MkdirTemp("", "flyctl-dockerfile-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temporary Dockerfile directory: %w", err)
	}
	cleanup = func() error { return os.RemoveAll(tempDir) }

	path = filepath.Join(tempDir, "Dockerfile")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}

		return "", nil, fmt.Errorf("creating temporary Dockerfile: %w", err)
	}

	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("closing temporary Dockerfile: %w", closeErr)
		}
		if err != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("removing temporary Dockerfile directory: %w", cleanupErr))
			}
			path = ""
			cleanup = nil
		}
	}()

	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if copyErr != nil {
		err = fmt.Errorf("writing temporary Dockerfile: %w", copyErr)
	} else if written > maxBytes {
		err = fmt.Errorf("downloading Dockerfile: response exceeds %d-byte limit", maxBytes)
	}

	return
}
