package mock

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

var _ uiexutil.Client = (*UiexClient)(nil)

// UiexClient implements the uiexutil.Client interface for testing
type UiexClient struct {
	ListOrganizationsFunc      func(ctx context.Context, admin bool) ([]uiex.Organization, error)
	GetOrganizationFunc        func(ctx context.Context, orgSlug string) (*uiex.Organization, error)
	PromoteMachineEgressIPFunc func(ctx context.Context, appName string, egressIP string) error

	CreateBuildFunc                        func(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error)
	FinishBuildFunc                        func(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error)
	EnsureDepotBuilderFunc                 func(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error)
	CreateFlyManagedBuilderFunc            func(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error)
	GetAllAppsCurrentReleaseTimestampsFunc func(ctx context.Context) (*map[string]time.Time, error)
	ListReleasesFunc                       func(ctx context.Context, appName string, count int) ([]uiex.Release, error)
	GetCurrentReleaseFunc                  func(ctx context.Context, appName string) (*uiex.Release, error)
	CreateReleaseFunc                      func(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error)
	UpdateReleaseFunc                      func(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error)
}

func (m *UiexClient) BaseURL() *url.URL {
	return nil
}

func (m *UiexClient) HTTPClient() *http.Client {
	return nil
}

func (m *UiexClient) ListOrganizations(ctx context.Context, admin bool) ([]uiex.Organization, error) {
	if m.ListOrganizationsFunc != nil {
		return m.ListOrganizationsFunc(ctx, admin)
	}

	return []uiex.Organization{}, nil
}

func (m *UiexClient) GetOrganization(ctx context.Context, orgSlug string) (*uiex.Organization, error) {
	if m.GetOrganizationFunc != nil {
		return m.GetOrganizationFunc(ctx, orgSlug)
	}

	return &uiex.Organization{Slug: orgSlug}, nil
}

func (m *UiexClient) PromoteMachineEgressIP(ctx context.Context, appName string, egressIP string) error {
	if m.PromoteMachineEgressIPFunc != nil {
		return m.PromoteMachineEgressIPFunc(ctx, appName, egressIP)
	}

	return nil
}

func (m *UiexClient) CreateBuild(ctx context.Context, in uiex.CreateBuildRequest) (*uiex.BuildResponse, error) {
	if m.CreateBuildFunc != nil {
		return m.CreateBuildFunc(ctx, in)
	}

	return &uiex.BuildResponse{}, nil
}

func (m *UiexClient) FinishBuild(ctx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error) {
	if m.FinishBuildFunc != nil {
		return m.FinishBuildFunc(ctx, in)
	}

	return &uiex.BuildResponse{}, nil
}

func (m *UiexClient) EnsureDepotBuilder(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error) {
	if m.EnsureDepotBuilderFunc != nil {
		return m.EnsureDepotBuilderFunc(ctx, in)
	}

	return &uiex.EnsureDepotBuilderResponse{}, nil
}

func (m *UiexClient) CreateFlyManagedBuilder(ctx context.Context, orgSlug string, region string) (uiex.CreateFlyManagedBuilderResponse, error) {
	if m.CreateFlyManagedBuilderFunc != nil {
		return m.CreateFlyManagedBuilderFunc(ctx, orgSlug, region)
	}

	return uiex.CreateFlyManagedBuilderResponse{}, nil
}

func (m *UiexClient) GetAllAppsCurrentReleaseTimestamps(ctx context.Context) (*map[string]time.Time, error) {
	if m.GetAllAppsCurrentReleaseTimestampsFunc != nil {
		return m.GetAllAppsCurrentReleaseTimestampsFunc(ctx)
	}

	return &map[string]time.Time{}, nil
}

func (m *UiexClient) ListReleases(ctx context.Context, appName string, count int) ([]uiex.Release, error) {
	if m.ListReleasesFunc != nil {
		return m.ListReleasesFunc(ctx, appName, count)
	}

	return []uiex.Release{}, nil
}

func (m *UiexClient) GetCurrentRelease(ctx context.Context, appName string) (*uiex.Release, error) {
	if m.GetCurrentReleaseFunc != nil {
		return m.GetCurrentReleaseFunc(ctx, appName)
	}

	return &uiex.Release{}, nil
}

func (m *UiexClient) CreateRelease(ctx context.Context, req uiex.CreateReleaseRequest) (*uiex.Release, error) {
	if m.CreateReleaseFunc != nil {
		return m.CreateReleaseFunc(ctx, req)
	}

	return &uiex.Release{}, nil
}

func (m *UiexClient) UpdateRelease(ctx context.Context, releaseID, status string, metadata any) (*uiex.Release, error) {
	if m.UpdateReleaseFunc != nil {
		return m.UpdateReleaseFunc(ctx, releaseID, status, metadata)
	}

	return &uiex.Release{}, nil
}
