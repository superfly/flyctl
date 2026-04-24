package uiexutil

import (
	"context"
	"time"

	"github.com/superfly/flyctl/internal/uiex"
)

type Client interface {
	// Basic
	ListOrganizations(ctx context.Context, admin bool) ([]uiex.Organization, error)
	GetOrganization(ctx context.Context, orgSlug string) (*uiex.Organization, error)

	// Egress IPs
	PromoteMachineEgressIP(ctx context.Context, appName string, egressIP string) error

	// Builders
	CreateBuild(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error)
	FinishBuild(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error)
	EnsureDepotBuilder(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error)
	CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error)

	// Releases
	GetAllAppsCurrentReleaseTimestamps(ctx context.Context) (*map[string]time.Time, error)
	ListReleases(ctx context.Context, appName string, count int) ([]uiex.Release, error)
	GetCurrentRelease(ctx context.Context, appName string) (*uiex.Release, error)
	CreateRelease(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error)
	UpdateRelease(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error)
}

type contextKey struct{}

var clientContextKey = &contextKey{}

// NewContextWithClient derives a Context that carries c from ctx.
func NewContextWithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the Client ctx carries.
func ClientFromContext(ctx context.Context) Client {
	c, _ := ctx.Value(clientContextKey).(Client)

	return c
}
