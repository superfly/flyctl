package scan

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/flyutil"
)

var scantronUrl = "https://scantron.fly.dev"

var httpClient = &http.Client{
	Timeout: time.Second * 3,
}

// scantron makes a request to the scantron service for an app's image.
// It returns a vulnerability scan if sbomOnly is false, or an SBOM if sbomOnly is true.
func scantron(ctx context.Context, apiClient flyutil.Client, app *fly.AppCompact, machine *fly.Machine, sbomOnly bool) (*http.Response, error) {
	if val := os.Getenv("FLY_SCANTRON"); val != "" {
		scantronUrl = val
	}

	img := machine.ImageRef
	url := fmt.Sprintf("%s/%s/%s@%s", scantronUrl, img.Registry, img.Repository, img.Digest)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if sbomOnly {
		req.Header.Set("Accept", "application/spdx+json")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	if err := addAuth(ctx, apiClient, app.Organization.ID, app.ID, req); err != nil {
		return nil, fmt.Errorf("failed to create scanner token: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching data from scantron: %w", err)
	}
	return res, nil
}
