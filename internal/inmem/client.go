package inmem

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"

	genq "github.com/Khan/genqlient/graphql"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/graphql"
)

var DefaultUser = fly.User{
	ID:              "USER1",
	Name:            "TestUser",
	Email:           "test@fly.dev",
	EnablePaidHobby: false,
}

var _ flyutil.Client = (*Client)(nil)

type Client struct {
	server *Server

	CurrentUser *fly.User
}

func NewClient(s *Server) *Client {
	u := DefaultUser

	return &Client{
		server:      s,
		CurrentUser: &u,
	}
}

func (m *Client) AllocateEgressIPAddress(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error) {
	panic("TODO")
}

func (m *Client) AppNameAvailable(ctx context.Context, appName string) (bool, error) {
	panic("TODO")
}

func (m *Client) AttachPostgresCluster(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error) {
	panic("TODO")
}

func (m *Client) Authenticated() bool {
	return m.CurrentUser != nil
}

func (m *Client) CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error) {
	panic("TODO")
}

func (m *Client) ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error) {
	panic("TODO")
}

func (m *Client) CreateDoctorUrl(ctx context.Context) (putUrl string, err error) {
	panic("TODO")
}

func (m *Client) CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error) {
	panic("TODO")
}

func (m *Client) CreateWireGuardPeer(ctx context.Context, orgID string, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error) {
	panic("TODO")
}

func (m *Client) DeleteOrganization(ctx context.Context, id string) (deletedid string, err error) {
	panic("TODO")
}

func (m *Client) DeleteOrganizationMembership(ctx context.Context, orgId, userId string) (string, string, error) {
	panic("TODO")
}

func (m *Client) DetachPostgresCluster(ctx context.Context, input fly.DetachPostgresClusterInput) error {
	panic("TODO")
}

func (m *Client) EnablePostgresConsul(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error) {
	panic("TODO")
}

func (m *Client) FinishBuild(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error) {
	build, err := m.server.FinishBuild(ctx, input.BuildId, input.Status)
	if err != nil {
		return nil, err
	}

	var resp fly.FinishBuildResponse
	resp.FinishBuild.Id = build.ID
	resp.FinishBuild.Status = build.Status

	return &resp, nil
}

func (m *Client) GetApp(ctx context.Context, appName string) (*fly.App, error) {
	panic("TODO")
}

func (m *Client) GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	app := m.server.apps[appName]
	if app == nil {
		return nil, fmt.Errorf("app not found: %q", appName) // TODO: Match actual error
	}

	return app.Compact(), nil
}

func (m *Client) GetAppHostIssues(ctx context.Context, appName string) ([]fly.HostIssue, error) {
	panic("TODO")
}

func (m *Client) GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error) {
	panic("TODO")
}

func (m *Client) GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
	panic("TODO")
}

func (m *Client) GetAppNameFromVolume(ctx context.Context, volID string) (*string, error) {
	panic("TODO")
}

func (m *Client) GetAppReleasesMachines(ctx context.Context, appName, status string, limit int) ([]fly.Release, error) {
	panic("TODO")
}

func (m *Client) GetAppSecrets(ctx context.Context, appName string) ([]fly.Secret, error) {
	panic("TODO")
}

func (m *Client) GetApps(ctx context.Context, role *string) ([]fly.App, error) {
	panic("TODO")
}

func (m *Client) GetCurrentUser(ctx context.Context) (*fly.User, error) {
	return m.CurrentUser, nil
}

func (m *Client) GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error) {
	panic("TODO")
}

func (m *Client) GetEgressIPAddresses(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error) {
	panic("TODO")
}

func (m *Client) GetLatestImageDetails(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error) {
	panic("TODO")
}

func (m *Client) GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error) {
	panic("TODO")
}

func (m *Client) GetLoggedCertificates(ctx context.Context, slug string) ([]fly.LoggedCertificate, error) {
	panic("TODO")
}

func (m *Client) GetMachine(ctx context.Context, machineId string) (*fly.GqlMachine, error) {
	panic("TODO")
}

func (m *Client) GetOrgLimitedAccessTokens(ctx context.Context, orgSlug string) ([]fly.LimitedAccessToken, error) {
	panic("TODO")
}

func (m *Client) GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetAllowedReplaySourceOrgSlugs(ctx context.Context, slug string) ([]string, error) {
	panic("TODO")
}

func (m *Client) AddAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, sourceOrgSlugs []string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) RemoveAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, orgSlugsToRemove []string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetAllowAllCrossNetworkReplays(ctx context.Context, slug string) (bool, error) {
	panic("TODO")
}

func (m *Client) SetAllowAllCrossNetworkReplays(ctx context.Context, orgSlug string, allow bool) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error) {
	panic("TODO")
}

func (m *Client) GenqClient() genq.Client {
	panic("TODO")
}

func (m *Client) IssueSSHCertificate(ctx context.Context, orgID string, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error) {
	panic("TODO")
}

func (m *Client) ListPostgresClusterAttachments(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error) {
	panic("TODO")
}

func (m *Client) Logger() fly.Logger {
	panic("TODO")
}

func (m *Client) MoveApp(ctx context.Context, appName string, orgID string) (*fly.App, error) {
	panic("TODO")
}

func (m *Client) NewRequest(q string) *graphql.Request {
	panic("TODO")
}

func (m *Client) PlatformRegions(ctx context.Context) ([]fly.Region, *fly.Region, error) {
	panic("TODO")
}

func (m *Client) ReleaseEgressIPAddress(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error) {
	panic("TODO")
}

func (m *Client) RemoveWireGuardPeer(ctx context.Context, orgID string, name string) error {
	panic("TODO")
}

func (m *Client) ResolveImageForApp(ctx context.Context, appName, imageRef string) (*fly.Image, error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	image := m.server.images[imageKey{appName, imageRef}]
	if image == nil {
		return nil, fmt.Errorf("image not found for app %q: %s", appName, imageRef)
	}

	return image, nil
}

func (m *Client) RevokeLimitedAccessToken(ctx context.Context, id string) error {
	panic("TODO")
}

func (m *Client) Run(req *graphql.Request) (fly.Query, error) {
	panic("TODO")
}

func (m *Client) RunWithContext(ctx context.Context, req *graphql.Request) (fly.Query, error) {
	panic("TODO")
}

func (m *Client) SetGenqClient(client genq.Client) {
	panic("TODO")
}

func (m *Client) SetSecrets(ctx context.Context, appName string, secrets map[string]string) (*fly.Release, error) {
	panic("TODO")
}

func (m *Client) UnsetSecrets(ctx context.Context, appName string, keys []string) (*fly.Release, error) {
	panic("TODO")
}

func (m *Client) ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error) {
	panic("TODO")
}
