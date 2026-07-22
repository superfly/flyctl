package imgsrc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
		return "", nil, fmt.Errorf("creating Dockerfile request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("downloading Dockerfile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, fmt.Errorf("downloading Dockerfile: unexpected HTTP status %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return "", nil, fmt.Errorf("downloading Dockerfile: response exceeds %d-byte limit", maxBytes)
	}

	f, err := os.CreateTemp("", "flyctl-dockerfile-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temporary Dockerfile: %w", err)
	}

	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
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
