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

func (m *Client) AddCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error) {
	panic("TODO")
}

func (m *Client) AllocateIPAddress(ctx context.Context, appName string, addrType string, region string, org *fly.Organization, network string) (*fly.IPAddress, error) {
	panic("TODO")
}

func (m *Client) AllocateSharedIPAddress(ctx context.Context, appName string) (net.IP, error) {
	panic("TODO")
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

func (m *Client) CheckAppCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error) {
	panic("TODO")
}

func (m *Client) CheckDomain(ctx context.Context, name string) (*fly.CheckDomainResult, error) {
	panic("TODO")
}

func (m *Client) ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error) {
	panic("TODO")
}

func (m *Client) CreateAndRegisterDomain(organizationID string, name string) (*fly.Domain, error) {
	panic("TODO")
}

func (m *Client) CreateApp(ctx context.Context, input fly.CreateAppInput) (*fly.App, error) {
	panic("TODO")
}

func (m *Client) CreateBuild(ctx context.Context, input fly.CreateBuildInput) (*fly.CreateBuildResponse, error) {
	build, err := m.server.CreateBuild(ctx, input.AppName)
	if err != nil {
		return nil, err
	}

	var resp fly.CreateBuildResponse
	resp.CreateBuild.Id = build.ID
	resp.CreateBuild.Status = build.Status
	return &resp, nil
}

func (m *Client) CreateDelegatedWireGuardToken(ctx context.Context, org *fly.Organization, name string) (*fly.DelegatedWireGuardToken, error) {
	panic("TODO")
}

func (m *Client) CreateDoctorUrl(ctx context.Context) (putUrl string, err error) {
	panic("TODO")
}

func (m *Client) CreateDomain(organizationID string, name string) (*fly.Domain, error) {
	panic("TODO")
}

func (m *Client) CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error) {
	panic("TODO")
}

func (m *Client) CreateRelease(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error) {
	release, err := m.server.CreateRelease(ctx, input.AppId, input.ClientMutationId, input.Image, input.PlatformVersion, string(input.Strategy))
	if err != nil {
		return nil, err
	}

	var resp fly.CreateReleaseResponse
	resp.CreateRelease.Release.Id = release.ID
	resp.CreateRelease.Release.Version = release.Version
	return &resp, nil
}

func (m *Client) CreateWireGuardPeer(ctx context.Context, org *fly.Organization, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error) {
	panic("TODO")
}

func (m *Client) DeleteApp(ctx context.Context, appName string) error {
	panic("TODO")
}

func (m *Client) DeleteCertificate(ctx context.Context, appName, hostname string) (*fly.DeleteCertificatePayload, error) {
	panic("TODO")
}

func (m *Client) DeleteDelegatedWireGuardToken(ctx context.Context, org *fly.Organization, name, token *string) error {
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

func (m *Client) EnsureRemoteBuilder(ctx context.Context, orgID, appName, region string) (*fly.GqlMachine, *fly.App, error) {
	panic("TODO")
}

func (m *Client) EnsureDepotRemoteBuilder(ctx context.Context, input *fly.EnsureDepotRemoteBuilderInput) (*fly.EnsureDepotRemoteBuilderResponse, error) {
	panic("TODO")
}

func (m *Client) ExportDNSRecords(ctx context.Context, domainId string) (string, error) {
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

func (m *Client) GetAppBasic(ctx context.Context, appName string) (*fly.AppBasic, error) {
	panic("TODO")
}

func (m *Client) GetAppCertificates(ctx context.Context, appName string) ([]fly.AppCertificateCompact, error) {
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

func (m *Client) GetAppCurrentReleaseMachines(ctx context.Context, appName string) (*fly.Release, error) {
	panic("TODO")
}

func (m *Client) GetAppHostIssues(ctx context.Context, appName string) ([]fly.HostIssue, error) {
	panic("TODO")
}

func (m *Client) GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error) {
	panic("TODO")
}

func (m *Client) GetAppRemoteBuilder(ctx context.Context, appName string) (*fly.App, error) {
	panic("TODO")
}

func (m *Client) GetDeployerAppByOrg(ctx context.Context, orgID string) (*fly.App, error) {
	panic("TODO")
}

func (m *Client) GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
	panic("TODO")
}

func (m *Client) GetAppNameFromVolume(ctx context.Context, volID string) (*string, error) {
	panic("TODO")
}

func (m *Client) GetAppNameStateFromVolume(ctx context.Context, volID string) (*string, *string, error) {
	panic("TODO")
}

func (m *Client) GetAppNetwork(ctx context.Context, appName string) (*string, error) {
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

func (m *Client) GetAppsForOrganization(ctx context.Context, orgID string) ([]fly.App, error) {
	panic("TODO")
}

func (m *Client) GetCurrentUser(ctx context.Context) (*fly.User, error) {
	return m.CurrentUser, nil
}

func (m *Client) GetDNSRecords(ctx context.Context, domainName string) ([]*fly.DNSRecord, error) {
	panic("TODO")
}

func (m *Client) GetDelegatedWireGuardTokens(ctx context.Context, slug string) ([]*fly.DelegatedWireGuardTokenHandle, error) {
	panic("TODO")
}

func (m *Client) GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error) {
	panic("TODO")
}

func (m *Client) GetDomain(ctx context.Context, name string) (*fly.Domain, error) {
	panic("TODO")
}

func (m *Client) GetDomains(ctx context.Context, organizationSlug string) ([]*fly.Domain, error) {
	panic("TODO")
}

func (m *Client) GetIPAddresses(ctx context.Context, appName string) ([]fly.IPAddress, error) {
	return nil, nil // TODO
}

func (c *Client) GetEgressIPAddresses(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error) {
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

func (m *Client) GetNearestRegion(ctx context.Context) (*fly.Region, error) {
	panic("TODO")
}

func (m *Client) GetOrganizationByApp(ctx context.Context, appName string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetOrganizationBySlug(ctx context.Context, slug string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetOrganizationRemoteBuilderBySlug(ctx context.Context, slug string) (*fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
	panic("TODO")
}

func (m *Client) GetSnapshotsFromVolume(ctx context.Context, volID string) ([]fly.VolumeSnapshot, error) {
	panic("TODO")
}

func (m *Client) GetWireGuardPeer(ctx context.Context, slug, name string) (*fly.WireGuardPeer, error) {
	panic("TODO")
}

func (m *Client) GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error) {
	panic("TODO")
}

func (m *Client) GenqClient() genq.Client {
	panic("TODO")
}

func (m *Client) LatestImage(ctx context.Context, appName string) (string, error) {
	panic("TODO")
}

func (m *Client) ImportDNSRecords(ctx context.Context, domainId string, zonefile string) ([]fly.ImportDnsWarning, []fly.ImportDnsChange, error) {
	panic("TODO")
}

func (m *Client) IssueSSHCertificate(ctx context.Context, org fly.OrganizationImpl, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error) {
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

func (m *Client) ReleaseIPAddress(ctx context.Context, appName string, ip string) error {
	panic("TODO")
}

func (m *Client) RemoveWireGuardPeer(ctx context.Context, org *fly.Organization, name string) error {
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

func (m *Client) UpdateRelease(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error) {
	if err := m.server.UpdateRelease(ctx, input.ReleaseId, input.ClientMutationId, input.Status); err != nil {
		return nil, err
	}

	var resp fly.UpdateReleaseResponse
	resp.UpdateRelease.Release.Id = input.ReleaseId
	return &resp, nil
}

func (m *Client) UnsetSecrets(ctx context.Context, appName string, keys []string) (*fly.Release, error) {
	panic("TODO")
}

func (m *Client) ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error) {
	panic("TODO")
}
