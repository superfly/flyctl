package deploy

import (
	"context"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/logs"
)

// webClient is a subset of web API that is needed for the deploy package.
type webClient interface {
	AddCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error)

	LatestImage(ctx context.Context, appName string) (string, error)

	CreateRelease(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error)
	UpdateRelease(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error)

	GetOrganizationBySlug(ctx context.Context, slug string) (*fly.Organization, error)

	logs.WebClient
	blueGreenWebClient
}
