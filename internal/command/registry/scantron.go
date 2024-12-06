package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"

	"github.com/superfly/flyctl/internal/buildinfo"
)

const (
	scantronTokenLife  = "5m"
	scantronTokenName  = "ScantronToken"
	scantronDefaultUrl = "https://scantron.fly.dev"
)

var httpClient = &http.Client{
	Timeout: time.Second * 15,
}

func imageRefPath(imgRef *fly.MachineImageRef) string {
	return fmt.Sprintf("%s/%s@%s", imgRef.Registry, imgRef.Repository, imgRef.Digest)
}

// scantronReq requests information about imgPath from scanTron using token.
// The `accept` parameter is used as a header, which indicates which information
// scantron should serve up.
func scantronReq(ctx context.Context, imgPath, token, accept string) (*http.Response, error) {
	scantronUrl := scantronDefaultUrl
	if val := os.Getenv("FLY_SCANTRON"); val != "" {
		scantronUrl = val
	}

	url := fmt.Sprintf("%s/%s", scantronUrl, imgPath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create scantron HTTP request: %w", err)
	}

	req.Header.Set("User-Agent", buildinfo.UserAgent())
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", fly.AuthorizationHeader(token))
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching data from scantron: %w", err)
	}
	return res, nil
}

func scantronSbomReq(ctx context.Context, imgPath, token string) (*http.Response, error) {
	return scantronReq(ctx, imgPath, token, "application/spdx+json")
}

func scantronVulnscanReq(ctx context.Context, imgPath, token string) (*http.Response, error) {
	return scantronReq(ctx, imgPath, token, "application/json")
}

func scantronFilesReq(ctx context.Context, imgPath, token string) (*http.Response, error) {
	return scantronReq(ctx, imgPath+"?files=1", token, "application/json")
}

type Scan struct {
	SchemaVersion int
	CreatedAt     string
	// Metadata
	Results []ScanResult
}

type ScanResult struct {
	Target          string
	Type            string
	Vulnerabilities []ScanVuln
}

type ScanVuln struct {
	VulnerabilityID  string
	PkgName          string
	InstalledVersion string
	Status           string
	Title            string
	Description      string
	Severity         string
}

type ErrUnsupportedPath string

func (e ErrUnsupportedPath) Error() string {
	return fmt.Sprintf("Unsupported image path %q", string(e))
}

func getVulnScan(ctx context.Context, imgPath, token string) (*Scan, error) {
	res, err := scantronVulnscanReq(ctx, imgPath, token)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close() // skipcq: GO-S2307

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == 422 {
			return nil, ErrUnsupportedPath(imgPath)
		}

		buf := make([]byte, 512)
		n, _ := res.Body.Read(buf)
		msg := strings.TrimSuffix(string(buf[:n]), "\n")
		if msg == "" {
			msg = "undetermined"
		}
		return nil, fmt.Errorf("status code %d: %q", res.StatusCode, msg)
	}

	scan := &Scan{}
	if err = json.NewDecoder(res.Body).Decode(scan); err != nil {
		return nil, fmt.Errorf("reading scan results: %w", err)
	}
	if scan.SchemaVersion != 2 {
		return nil, fmt.Errorf("unknown scan schema %d", scan.SchemaVersion)
	}
	return scan, nil
}

func makeScantronToken(ctx context.Context, orgId string) (string, error) {
	resp, err := makeToken(ctx, scantronTokenName, orgId, scantronTokenLife, "registry_token", &gql.LimitedAccessTokenOptions{})
	if err != nil {
		return "", err
	}

	token := resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	return token, nil
}
