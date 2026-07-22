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
)

const (
	dockerfileDownloadTimeout = 30 * time.Second
	maxDockerfileSizeBytes    = 10 << 20
)

// IsDockerfileURL reports whether path is an HTTP(S) URL.
func IsDockerfileURL(path string) bool {
	u, err := url.Parse(path)
	if err != nil || u.Host == "" {
		return false
	}

	return strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")
}

func redactDockerfileURL(path string) string {
	if !IsDockerfileURL(path) {
		return path
	}

	u, err := url.Parse(path)
	if err != nil {
		return path
	}

	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""

	return u.String()
}

func redactDockerfileRequestError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}

	redacted := *urlErr
	redacted.URL = redactDockerfileURL(urlErr.URL)

	return &redacted
}

func materializeDockerfile(ctx context.Context, path string) (localPath string, cleanup func(), err error) {
	if !IsDockerfileURL(path) {
		return path, func() {}, nil
	}

	return downloadDockerfile(ctx, path, dockerfileDownloadTimeout, maxDockerfileSizeBytes)
}

func downloadDockerfile(ctx context.Context, dockerfileURL string, timeout time.Duration, maxBytes int64) (path string, cleanup func(), err error) {
	downloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, dockerfileURL, http.NoBody)
	if err != nil {
		return "", nil, fmt.Errorf("creating Dockerfile request: %w", redactDockerfileRequestError(err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("downloading Dockerfile: %w", redactDockerfileRequestError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, fmt.Errorf("downloading Dockerfile: unexpected HTTP status %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return "", nil, fmt.Errorf("downloading Dockerfile: response exceeds %d-byte limit", maxBytes)
	}

	tempDir, err := os.MkdirTemp("", "flyctl-dockerfile-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temporary Dockerfile directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tempDir) }

	path = filepath.Join(tempDir, "Dockerfile")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("creating temporary Dockerfile: %w", err)
	}

	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("closing temporary Dockerfile: %w", closeErr)
		}
		if err != nil {
			cleanup()
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
