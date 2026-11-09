package mock

import (
	"context"
	"crypto/ed25519"
	"net"

	genq "github.com/Khan/genqlient/graphql"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/graphql"
)

var _ flyutil.Client = (*Client)(nil)

type Client struct {
	AllocateEgressIPAddressFunc        func(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error)
	AppNameAvailableFunc               func(ctx context.Context, appName string) (bool, error)
	AttachPostgresClusterFunc          func(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error)
	AuthenticatedFunc                  func() bool
	CanPerformBluegreenDeploymentFunc  func(ctx context.Context, appName string) (bool, error)
	ClosestWireguardGatewayRegionFunc  func(ctx context.Context) (*fly.Region, error)
	CreateDoctorUrlFunc                func(ctx context.Context) (putUrl string, err error)
	CreateOrganizationFunc             func(ctx context.Context, organizationname string) (*fly.Organization, error)
	CreateOrganizationInviteFunc       func(ctx context.Context, id, email string) (*fly.Invitation, error)
	CreateWireGuardPeerFunc            func(ctx context.Context, orgID string, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error)
	DeleteOrganizationFunc             func(ctx context.Context, id string) (deletedid string, err error)
	DeleteOrganizationMembershipFunc   func(ctx context.Context, orgId, userId string) (string, string, error)
	DetachPostgresClusterFunc          func(ctx context.Context, input fly.DetachPostgresClusterInput) error
	EnablePostgresConsulFunc           func(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error)
	FinishBuildFunc                    func(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error)
	GetAppFunc                         func(ctx context.Context, appName string) (*fly.App, error)
	GetAppCompactFunc                  func(ctx context.Context, appName string) (*fly.AppCompact, error)
	GetAppHostIssuesFunc               func(ctx context.Context, appName string) ([]fly.HostIssue, error)
	GetAppLimitedAccessTokensFunc      func(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error)
	GetAppLogsFunc                     func(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error)
	GetAppNameFromVolumeFunc           func(ctx context.Context, volID string) (*string, error)
	GetAppReleasesMachinesFunc         func(ctx context.Context, appName, status string, limit int) ([]fly.Release, error)
	GetAppSecretsFunc                  func(ctx context.Context, appName string) ([]fly.Secret, error)
	GetAppsFunc                        func(ctx context.Context, role *string) ([]fly.App, error)
	GetCurrentUserFunc                 func(ctx context.Context) (*fly.User, error)
	GetDetailedOrganizationBySlugFunc  func(ctx context.Context, slug string) (*fly.OrganizationDetails, error)
	GetEgressIPAddressesFunc           func(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error)
	GetLatestImageDetailsFunc          func(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error)
	GetLatestImageTagFunc              func(ctx context.Context, repository string, snapshotId *string) (string, error)
	GetLoggedCertificatesFunc          func(ctx context.Context, slug string) ([]fly.LoggedCertificate, error)
	GetMachineFunc                     func(ctx context.Context, machineId string) (*fly.GqlMachine, error)
	GetOrgLimitedAccessTokensFunc      func(ctx context.Context, orgSlug string) ([]fly.LimitedAccessToken, error)
	GetOrganizationsFunc               func(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error)
	GetAllowedReplaySourceOrgSlugsFunc func(ctx context.Context, slug string) ([]string, error)
	AddAllowedReplaySourceOrgsFunc     func(ctx context.Context, orgSlug string, sourceOrgSlugs []string) (*fly.Organization, error)
	RemoveAllowedReplaySourceOrgsFunc  func(ctx context.Context, orgSlug string, orgSlugsToRemove []string) (*fly.Organization, error)
	GetAllowAllCrossNetworkReplaysFunc func(ctx context.Context, slug string) (bool, error)
	SetAllowAllCrossNetworkReplaysFunc func(ctx context.Context, orgSlug string, allow bool) (*fly.Organization, error)
	GetWireGuardPeersFunc              func(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error)
	GenqClientFunc                     func() genq.Client
	IssueSSHCertificateFunc            func(ctx context.Context, orgID string, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error)
	ListPostgresClusterAttachmentsFunc func(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error)
	LoggerFunc                         func() fly.Logger
	MoveAppFunc                        func(ctx context.Context, appName string, orgID string) (*fly.App, error)
	NewRequestFunc                     func(q string) *graphql.Request
	PlatformRegionsFunc                func(ctx context.Context) ([]fly.Region, *fly.Region, error)
	ReleaseEgressIPAddressFunc         func(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error)
	RemoveWireGuardPeerFunc            func(ctx context.Context, orgID string, name string) error
	ResolveImageForAppFunc             func(ctx context.Context, appName, imageRef string) (*fly.Image, error)
	RevokeLimitedAccessTokenFunc       func(ctx context.Context, id string) error
	RunFunc                            func(req *graphql.Request) (fly.Query, error)
	RunWithContextFunc                 func(ctx context.Context, req *graphql.Request) (fly.Query, error)
	SetGenqClientFunc                  func(client genq.Client)
	SetRemoteBuilderFunc               func(ctx context.Context, appName string) error
	SetSecretsFunc                     func(ctx context.Context, appName string, secrets map[string]string) (*fly.Release, error)
	UnsetSecretsFunc                   func(ctx context.Context, appName string, keys []string) (*fly.Release, error)
	ValidateWireGuardPeersFunc         func(ctx context.Context, peerIPs []string) (invalid []string, err error)
}

func (m *Client) AllocateEgressIPAddress(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error) {
	return m.AllocateEgressIPAddressFunc(ctx, appName, machineId)
}

func (m *Client) AppNameAvailable(ctx context.Context, appName string) (bool, error) {
	return m.AppNameAvailableFunc(ctx, appName)
}

func (m *Client) AttachPostgresCluster(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error) {
	return m.AttachPostgresClusterFunc(ctx, input)
}

func (m *Client) Authenticated() bool {
	return m.AuthenticatedFunc()
}

func (m *Client) CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error) {
	return m.CanPerformBluegreenDeploymentFunc(ctx, appName)
}

func (m *Client) ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error) {
	return m.ClosestWireguardGatewayRegionFunc(ctx)
}

func (m *Client) CreateDoctorUrl(ctx context.Context) (putUrl string, err error) {
	return m.CreateDoctorUrlFunc(ctx)
}

func (m *Client) CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error) {
	return m.CreateOrganizationFunc(ctx, organizationname)
}

func (m *Client) CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error) {
	return m.CreateOrganizationInviteFunc(ctx, id, email)
}

func (m *Client) CreateWireGuardPeer(ctx context.Context, orgID string, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error) {
	return m.CreateWireGuardPeerFunc(ctx, orgID, region, name, pubkey, network)
}

func (m *Client) DeleteOrganization(ctx context.Context, id string) (deletedid string, err error) {
	return m.DeleteOrganizationFunc(ctx, id)
}

func (m *Client) DeleteOrganizationMembership(ctx context.Context, orgId, userId string) (string, string, error) {
	return m.DeleteOrganizationMembershipFunc(ctx, orgId, userId)
}

func (m *Client) DetachPostgresCluster(ctx context.Context, input fly.DetachPostgresClusterInput) error {
	return m.DetachPostgresClusterFunc(ctx, input)
}

func (m *Client) EnablePostgresConsul(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error) {
	return m.EnablePostgresConsulFunc(ctx, appName)
}

func (m *Client) FinishBuild(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error) {
	return m.FinishBuildFunc(ctx, input)
}

func (m *Client) GetApp(ctx context.Context, appName string) (*fly.App, error) {
	return m.GetAppFunc(ctx, appName)
}

func (m *Client) GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error) {
	return m.GetAppCompactFunc(ctx, appName)
}

func (m *Client) GetAppHostIssues(ctx context.Context, appName string) ([]fly.HostIssue, error) {
	return m.GetAppHostIssuesFunc(ctx, appName)
}

func (m *Client) GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error) {
	return m.GetAppLimitedAccessTokensFunc(ctx, appName)
}

func (m *Client) GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error) {
	return m.GetAppLogsFunc(ctx, appName, token, region, instanceID)
}

func (m *Client) GetAppNameFromVolume(ctx context.Context, volID string) (*string, error) {
	return m.GetAppNameFromVolumeFunc(ctx, volID)
}

func (m *Client) GetAppReleasesMachines(ctx context.Context, appName, status string, limit int) ([]fly.Release, error) {
	return m.GetAppReleasesMachinesFunc(ctx, appName, status, limit)
}

func (m *Client) GetAppSecrets(ctx context.Context, appName string) ([]fly.Secret, error) {
	return m.GetAppSecretsFunc(ctx, appName)
}

func (m *Client) GetApps(ctx context.Context, role *string) ([]fly.App, error) {
	return m.GetAppsFunc(ctx, role)
}

func (m *Client) GetCurrentUser(ctx context.Context) (*fly.User, error) {
	return m.GetCurrentUserFunc(ctx)
}

func (m *Client) GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error) {
	return m.GetDetailedOrganizationBySlugFunc(ctx, slug)
}

func (m *Client) GetEgressIPAddresses(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error) {
	return m.GetEgressIPAddressesFunc(ctx, appName)
}

func (m *Client) GetLatestImageDetails(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error) {
	return m.GetLatestImageDetailsFunc(ctx, image, flyVersion)
}

func (m *Client) GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error) {
	return m.GetLatestImageTagFunc(ctx, repository, snapshotId)
}

func (m *Client) GetLoggedCertificates(ctx context.Context, slug string) ([]fly.LoggedCertificate, error) {
	return m.GetLoggedCertificatesFunc(ctx, slug)
}

func (m *Client) GetMachine(ctx context.Context, machineId string) (*fly.GqlMachine, error) {
	return m.GetMachineFunc(ctx, machineId)
}

func (m *Client) GetOrgLimitedAccessTokens(ctx context.Context, orgSlug string) ([]fly.LimitedAccessToken, error) {
	return m.GetOrgLimitedAccessTokensFunc(ctx, orgSlug)
}

func (m *Client) GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
	return m.GetOrganizationsFunc(ctx, filters...)
}

func (m *Client) GetAllowedReplaySourceOrgSlugs(ctx context.Context, slug string) ([]string, error) {
	return m.GetAllowedReplaySourceOrgSlugsFunc(ctx, slug)
}

func (m *Client) AddAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, sourceOrgSlugs []string) (*fly.Organization, error) {
	return m.AddAllowedReplaySourceOrgsFunc(ctx, orgSlug, sourceOrgSlugs)
}

func (m *Client) RemoveAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, orgSlugsToRemove []string) (*fly.Organization, error) {
	return m.RemoveAllowedReplaySourceOrgsFunc(ctx, orgSlug, orgSlugsToRemove)
}

func (m *Client) GetAllowAllCrossNetworkReplays(ctx context.Context, slug string) (bool, error) {
	return m.GetAllowAllCrossNetworkReplaysFunc(ctx, slug)
}

func (m *Client) SetAllowAllCrossNetworkReplays(ctx context.Context, orgSlug string, allow bool) (*fly.Organization, error) {
	return m.SetAllowAllCrossNetworkReplaysFunc(ctx, orgSlug, allow)
}

func (m *Client) GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error) {
	return m.GetWireGuardPeersFunc(ctx, slug)
}

func (m *Client) GenqClient() genq.Client {
	return m.GenqClientFunc()
}

func (m *Client) IssueSSHCertificate(ctx context.Context, orgID string, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error) {
	return m.IssueSSHCertificateFunc(ctx, orgID, principals, appNames, valid_hours, publicKey)
}

func (m *Client) ListPostgresClusterAttachments(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error) {
	return m.ListPostgresClusterAttachmentsFunc(ctx, appName, postgresAppName)
}

func (m *Client) Logger() fly.Logger {
	return m.LoggerFunc()
}

func (m *Client) MoveApp(ctx context.Context, appName string, orgID string) (*fly.App, error) {
	return m.MoveAppFunc(ctx, appName, orgID)
}

func (m *Client) NewRequest(q string) *graphql.Request {
	return m.NewRequestFunc(q)
}

func (m *Client) PlatformRegions(ctx context.Context) ([]fly.Region, *fly.Region, error) {
	return m.PlatformRegionsFunc(ctx)
}

func (m *Client) ReleaseEgressIPAddress(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error) {
	return m.ReleaseEgressIPAddressFunc(ctx, appName, machineID)
}

func (m *Client) RemoveWireGuardPeer(ctx context.Context, orgID string, name string) error {
	return m.RemoveWireGuardPeerFunc(ctx, orgID, name)
}

func (m *Client) ResolveImageForApp(ctx context.Context, appName, imageRef string) (*fly.Image, error) {
	return m.ResolveImageForAppFunc(ctx, appName, imageRef)
}

func (m *Client) RevokeLimitedAccessToken(ctx context.Context, id string) error {
	return m.RevokeLimitedAccessTokenFunc(ctx, id)
}

func (m *Client) Run(req *graphql.Request) (fly.Query, error) {
	return m.RunFunc(req)
}

func (m *Client) RunWithContext(ctx context.Context, req *graphql.Request) (fly.Query, error) {
	return m.RunWithContextFunc(ctx, req)
}

func (m *Client) SetGenqClient(client genq.Client) {
	m.SetGenqClientFunc(client)
}

func (m *Client) SetRemoteBuilder(ctx context.Context, appName string) error {
	return m.SetRemoteBuilderFunc(ctx, appName)
}

func (m *Client) SetSecrets(ctx context.Context, appName string, secrets map[string]string) (*fly.Release, error) {
	return m.SetSecretsFunc(ctx, appName, secrets)
}

func (m *Client) UnsetSecrets(ctx context.Context, appName string, keys []string) (*fly.Release, error) {
	return m.UnsetSecretsFunc(ctx, appName, keys)
}

func (m *Client) ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error) {
	return m.ValidateWireGuardPeersFunc(ctx, peerIPs)
}
