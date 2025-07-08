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
	AddCertificateFunc                     func(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error)
	AllocateIPAddressFunc                  func(ctx context.Context, appName string, addrType string, region string, org *fly.Organization, network string) (*fly.IPAddress, error)
	AllocateSharedIPAddressFunc            func(ctx context.Context, appName string) (net.IP, error)
	AllocateEgressIPAddressFunc            func(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error)
	AppNameAvailableFunc                   func(ctx context.Context, appName string) (bool, error)
	AttachPostgresClusterFunc              func(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error)
	AuthenticatedFunc                      func() bool
	CanPerformBluegreenDeploymentFunc      func(ctx context.Context, appName string) (bool, error)
	CheckAppCertificateFunc                func(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error)
	CheckDomainFunc                        func(ctx context.Context, name string) (*fly.CheckDomainResult, error)
	ClosestWireguardGatewayRegionFunc      func(ctx context.Context) (*fly.Region, error)
	CreateAndRegisterDomainFunc            func(organizationID string, name string) (*fly.Domain, error)
	CreateAppFunc                          func(ctx context.Context, input fly.CreateAppInput) (*fly.App, error)
	CreateBuildFunc                        func(ctx context.Context, input fly.CreateBuildInput) (*fly.CreateBuildResponse, error)
	CreateDelegatedWireGuardTokenFunc      func(ctx context.Context, org *fly.Organization, name string) (*fly.DelegatedWireGuardToken, error)
	CreateDoctorUrlFunc                    func(ctx context.Context) (putUrl string, err error)
	CreateDomainFunc                       func(organizationID string, name string) (*fly.Domain, error)
	CreateOrganizationFunc                 func(ctx context.Context, organizationname string) (*fly.Organization, error)
	CreateOrganizationInviteFunc           func(ctx context.Context, id, email string) (*fly.Invitation, error)
	CreateReleaseFunc                      func(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error)
	CreateWireGuardPeerFunc                func(ctx context.Context, org *fly.Organization, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error)
	DeleteAppFunc                          func(ctx context.Context, appName string) error
	DeleteCertificateFunc                  func(ctx context.Context, appName, hostname string) (*fly.DeleteCertificatePayload, error)
	DeleteDelegatedWireGuardTokenFunc      func(ctx context.Context, org *fly.Organization, name, token *string) error
	DeleteOrganizationFunc                 func(ctx context.Context, id string) (deletedid string, err error)
	DeleteOrganizationMembershipFunc       func(ctx context.Context, orgId, userId string) (string, string, error)
	DetachPostgresClusterFunc              func(ctx context.Context, input fly.DetachPostgresClusterInput) error
	EnablePostgresConsulFunc               func(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error)
	EnsureDepotRemoteBuilderFunc           func(ctx context.Context, input *fly.EnsureDepotRemoteBuilderInput) (*fly.EnsureDepotRemoteBuilderResponse, error)
	EnsureRemoteBuilderFunc                func(ctx context.Context, orgID, appName, region string) (*fly.GqlMachine, *fly.App, error)
	ExportDNSRecordsFunc                   func(ctx context.Context, domainId string) (string, error)
	FinishBuildFunc                        func(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error)
	GetAppFunc                             func(ctx context.Context, appName string) (*fly.App, error)
	GetAppRemoteBuilderFunc                func(ctx context.Context, appName string) (*fly.App, error)
	GetAppBasicFunc                        func(ctx context.Context, appName string) (*fly.AppBasic, error)
	GetAppCertificatesFunc                 func(ctx context.Context, appName string) ([]fly.AppCertificateCompact, error)
	GetAppCompactFunc                      func(ctx context.Context, appName string) (*fly.AppCompact, error)
	GetDeployerAppByOrgFunc                func(ctx context.Context, orgID string) (*fly.App, error)
	GetAppCurrentReleaseMachinesFunc       func(ctx context.Context, appName string) (*fly.Release, error)
	GetAppHostIssuesFunc                   func(ctx context.Context, appName string) ([]fly.HostIssue, error)
	GetAppLimitedAccessTokensFunc          func(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error)
	GetAppLogsFunc                         func(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error)
	GetAppNameFromVolumeFunc               func(ctx context.Context, volID string) (*string, error)
	GetAppNameStateFromVolumeFunc          func(ctx context.Context, volID string) (*string, *string, error)
	GetAppNetworkFunc                      func(ctx context.Context, appName string) (*string, error)
	GetAppReleasesMachinesFunc             func(ctx context.Context, appName, status string, limit int) ([]fly.Release, error)
	GetAppSecretsFunc                      func(ctx context.Context, appName string) ([]fly.Secret, error)
	GetAppsFunc                            func(ctx context.Context, role *string) ([]fly.App, error)
	GetAppsForOrganizationFunc             func(ctx context.Context, orgID string) ([]fly.App, error)
	GetCurrentUserFunc                     func(ctx context.Context) (*fly.User, error)
	GetDNSRecordsFunc                      func(ctx context.Context, domainName string) ([]*fly.DNSRecord, error)
	GetDelegatedWireGuardTokensFunc        func(ctx context.Context, slug string) ([]*fly.DelegatedWireGuardTokenHandle, error)
	GetDetailedOrganizationBySlugFunc      func(ctx context.Context, slug string) (*fly.OrganizationDetails, error)
	GetDomainFunc                          func(ctx context.Context, name string) (*fly.Domain, error)
	GetDomainsFunc                         func(ctx context.Context, organizationSlug string) ([]*fly.Domain, error)
	GetIPAddressesFunc                     func(ctx context.Context, appName string) ([]fly.IPAddress, error)
	GetEgressIPAddressesFunc               func(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error)
	GetLatestImageDetailsFunc              func(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error)
	GetLatestImageTagFunc                  func(ctx context.Context, repository string, snapshotId *string) (string, error)
	GetLoggedCertificatesFunc              func(ctx context.Context, slug string) ([]fly.LoggedCertificate, error)
	GetMachineFunc                         func(ctx context.Context, machineId string) (*fly.GqlMachine, error)
	GetNearestRegionFunc                   func(ctx context.Context) (*fly.Region, error)
	GetOrganizationBySlugFunc              func(ctx context.Context, slug string) (*fly.Organization, error)
	GetOrganizationRemoteBuilderBySlugFunc func(ctx context.Context, slug string) (*fly.Organization, error)
	GetOrganizationByAppFunc               func(ctx context.Context, appName string) (*fly.Organization, error)
	GetOrganizationsFunc                   func(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error)
	GetSnapshotsFromVolumeFunc             func(ctx context.Context, volID string) ([]fly.VolumeSnapshot, error)
	GetWireGuardPeerFunc                   func(ctx context.Context, slug, name string) (*fly.WireGuardPeer, error)
	GetWireGuardPeersFunc                  func(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error)
	GenqClientFunc                         func() genq.Client
	ImportDNSRecordsFunc                   func(ctx context.Context, domainId string, zonefile string) ([]fly.ImportDnsWarning, []fly.ImportDnsChange, error)
	IssueSSHCertificateFunc                func(ctx context.Context, org fly.OrganizationImpl, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error)
	LatestImageFunc                        func(ctx context.Context, appName string) (string, error)
	ListPostgresClusterAttachmentsFunc     func(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error)
	LoggerFunc                             func() fly.Logger
	MoveAppFunc                            func(ctx context.Context, appName string, orgID string) (*fly.App, error)
	NewRequestFunc                         func(q string) *graphql.Request
	PlatformRegionsFunc                    func(ctx context.Context) ([]fly.Region, *fly.Region, error)
	ReleaseIPAddressFunc                   func(ctx context.Context, appName string, ip string) error
	ReleaseEgressIPAddressFunc             func(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error)
	RemoveWireGuardPeerFunc                func(ctx context.Context, org *fly.Organization, name string) error
	ResolveImageForAppFunc                 func(ctx context.Context, appName, imageRef string) (*fly.Image, error)
	RevokeLimitedAccessTokenFunc           func(ctx context.Context, id string) error
	RunFunc                                func(req *graphql.Request) (fly.Query, error)
	RunWithContextFunc                     func(ctx context.Context, req *graphql.Request) (fly.Query, error)
	SetGenqClientFunc                      func(client genq.Client)
	SetRemoteBuilderFunc                   func(ctx context.Context, appName string) error
	SetSecretsFunc                         func(ctx context.Context, appName string, secrets map[string]string) (*fly.Release, error)
	UpdateReleaseFunc                      func(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error)
	UnsetSecretsFunc                       func(ctx context.Context, appName string, keys []string) (*fly.Release, error)
	ValidateWireGuardPeersFunc             func(ctx context.Context, peerIPs []string) (invalid []string, err error)
}

func (m *Client) AddCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error) {
	return m.AddCertificateFunc(ctx, appName, hostname)
}

func (m *Client) AllocateIPAddress(ctx context.Context, appName string, addrType string, region string, org *fly.Organization, network string) (*fly.IPAddress, error) {
	return m.AllocateIPAddressFunc(ctx, appName, addrType, region, org, network)
}

func (m *Client) AllocateSharedIPAddress(ctx context.Context, appName string) (net.IP, error) {
	return m.AllocateSharedIPAddressFunc(ctx, appName)
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

func (m *Client) CheckAppCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error) {
	return m.CheckAppCertificateFunc(ctx, appName, hostname)
}

func (m *Client) CheckDomain(ctx context.Context, name string) (*fly.CheckDomainResult, error) {
	return m.CheckDomainFunc(ctx, name)
}

func (m *Client) ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error) {
	return m.ClosestWireguardGatewayRegionFunc(ctx)
}

func (m *Client) CreateAndRegisterDomain(organizationID string, name string) (*fly.Domain, error) {
	return m.CreateAndRegisterDomainFunc(organizationID, name)
}

func (m *Client) CreateApp(ctx context.Context, input fly.CreateAppInput) (*fly.App, error) {
	return m.CreateAppFunc(ctx, input)
}

func (m *Client) CreateBuild(ctx context.Context, input fly.CreateBuildInput) (*fly.CreateBuildResponse, error) {
	return m.CreateBuildFunc(ctx, input)
}

func (m *Client) CreateDelegatedWireGuardToken(ctx context.Context, org *fly.Organization, name string) (*fly.DelegatedWireGuardToken, error) {
	return m.CreateDelegatedWireGuardTokenFunc(ctx, org, name)
}

func (m *Client) CreateDoctorUrl(ctx context.Context) (putUrl string, err error) {
	return m.CreateDoctorUrlFunc(ctx)
}

func (m *Client) CreateDomain(organizationID string, name string) (*fly.Domain, error) {
	return m.CreateDomainFunc(organizationID, name)
}

func (m *Client) CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error) {
	return m.CreateOrganizationFunc(ctx, organizationname)
}

func (m *Client) CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error) {
	return m.CreateOrganizationInviteFunc(ctx, id, email)
}

func (m *Client) CreateRelease(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error) {
	return m.CreateReleaseFunc(ctx, input)
}

func (m *Client) CreateWireGuardPeer(ctx context.Context, org *fly.Organization, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error) {
	return m.CreateWireGuardPeerFunc(ctx, org, region, name, pubkey, network)
}

func (m *Client) DeleteApp(ctx context.Context, appName string) error {
	return m.DeleteAppFunc(ctx, appName)
}

func (m *Client) DeleteCertificate(ctx context.Context, appName, hostname string) (*fly.DeleteCertificatePayload, error) {
	return m.DeleteCertificateFunc(ctx, appName, hostname)
}

func (m *Client) DeleteDelegatedWireGuardToken(ctx context.Context, org *fly.Organization, name, token *string) error {
	return m.DeleteDelegatedWireGuardTokenFunc(ctx, org, name, token)
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

func (m *Client) EnsureRemoteBuilder(ctx context.Context, orgID, appName, region string) (*fly.GqlMachine, *fly.App, error) {
	return m.EnsureRemoteBuilderFunc(ctx, orgID, appName, region)
}

func (m *Client) EnsureDepotRemoteBuilder(ctx context.Context, input *fly.EnsureDepotRemoteBuilderInput) (*fly.EnsureDepotRemoteBuilderResponse, error) {
	return m.EnsureDepotRemoteBuilderFunc(ctx, input)
}

func (m *Client) ExportDNSRecords(ctx context.Context, domainId string) (string, error) {
	return m.ExportDNSRecordsFunc(ctx, domainId)
}

func (m *Client) FinishBuild(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error) {
	return m.FinishBuildFunc(ctx, input)
}

func (m *Client) GetApp(ctx context.Context, appName string) (*fly.App, error) {
	return m.GetAppFunc(ctx, appName)
}

func (m *Client) GetAppRemoteBuilder(ctx context.Context, appName string) (*fly.App, error) {
	return m.GetAppRemoteBuilderFunc(ctx, appName)
}

func (m *Client) GetAppBasic(ctx context.Context, appName string) (*fly.AppBasic, error) {
	return m.GetAppBasicFunc(ctx, appName)
}

func (m *Client) GetAppCertificates(ctx context.Context, appName string) ([]fly.AppCertificateCompact, error) {
	return m.GetAppCertificatesFunc(ctx, appName)
}

func (m *Client) GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error) {
	return m.GetAppCompactFunc(ctx, appName)
}

func (m *Client) GetDeployerAppByOrg(ctx context.Context, orgID string) (*fly.App, error) {
	return m.GetDeployerAppByOrgFunc(ctx, orgID)
}

func (m *Client) GetAppCurrentReleaseMachines(ctx context.Context, appName string) (*fly.Release, error) {
	return m.GetAppCurrentReleaseMachinesFunc(ctx, appName)
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

func (m *Client) GetAppNameStateFromVolume(ctx context.Context, volID string) (*string, *string, error) {
	return m.GetAppNameStateFromVolumeFunc(ctx, volID)
}

func (m *Client) GetAppNetwork(ctx context.Context, appName string) (*string, error) {
	return m.GetAppNetworkFunc(ctx, appName)
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

func (m *Client) GetAppsForOrganization(ctx context.Context, orgID string) ([]fly.App, error) {
	return m.GetAppsForOrganizationFunc(ctx, orgID)
}

func (m *Client) GetCurrentUser(ctx context.Context) (*fly.User, error) {
	return m.GetCurrentUserFunc(ctx)
}

func (m *Client) GetDNSRecords(ctx context.Context, domainName string) ([]*fly.DNSRecord, error) {
	return m.GetDNSRecordsFunc(ctx, domainName)
}

func (m *Client) GetDelegatedWireGuardTokens(ctx context.Context, slug string) ([]*fly.DelegatedWireGuardTokenHandle, error) {
	return m.GetDelegatedWireGuardTokensFunc(ctx, slug)
}

func (m *Client) GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error) {
	return m.GetDetailedOrganizationBySlugFunc(ctx, slug)
}

func (m *Client) GetDomain(ctx context.Context, name string) (*fly.Domain, error) {
	return m.GetDomainFunc(ctx, name)
}

func (m *Client) GetDomains(ctx context.Context, organizationSlug string) ([]*fly.Domain, error) {
	return m.GetDomainsFunc(ctx, organizationSlug)
}

func (m *Client) GetIPAddresses(ctx context.Context, appName string) ([]fly.IPAddress, error) {
	return m.GetIPAddressesFunc(ctx, appName)
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

func (m *Client) GetNearestRegion(ctx context.Context) (*fly.Region, error) {
	return m.GetNearestRegionFunc(ctx)
}

func (m *Client) GetOrganizationBySlug(ctx context.Context, slug string) (*fly.Organization, error) {
	return m.GetOrganizationBySlugFunc(ctx, slug)
}

func (m *Client) GetOrganizationRemoteBuilderBySlug(ctx context.Context, slug string) (*fly.Organization, error) {
	return m.GetOrganizationRemoteBuilderBySlugFunc(ctx, slug)
}

func (m *Client) GetOrganizationByApp(ctx context.Context, appName string) (*fly.Organization, error) {
	return m.GetOrganizationByAppFunc(ctx, appName)
}

func (m *Client) GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
	return m.GetOrganizationsFunc(ctx, filters...)
}

func (m *Client) GetSnapshotsFromVolume(ctx context.Context, volID string) ([]fly.VolumeSnapshot, error) {
	return m.GetSnapshotsFromVolumeFunc(ctx, volID)
}

func (m *Client) GetWireGuardPeer(ctx context.Context, slug, name string) (*fly.WireGuardPeer, error) {
	return m.GetWireGuardPeerFunc(ctx, slug, name)
}

func (m *Client) GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error) {
	return m.GetWireGuardPeersFunc(ctx, slug)
}

func (m *Client) GenqClient() genq.Client {
	return m.GenqClientFunc()
}

func (m *Client) LatestImage(ctx context.Context, appName string) (string, error) {
	return m.LatestImageFunc(ctx, appName)
}

func (m *Client) ImportDNSRecords(ctx context.Context, domainId string, zonefile string) ([]fly.ImportDnsWarning, []fly.ImportDnsChange, error) {
	return m.ImportDNSRecordsFunc(ctx, domainId, zonefile)
}

func (m *Client) IssueSSHCertificate(ctx context.Context, org fly.OrganizationImpl, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error) {
	return m.IssueSSHCertificateFunc(ctx, org, principals, appNames, valid_hours, publicKey)
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

func (m *Client) ReleaseIPAddress(ctx context.Context, appName string, ip string) error {
	return m.ReleaseIPAddressFunc(ctx, appName, ip)
}

func (m *Client) RemoveWireGuardPeer(ctx context.Context, org *fly.Organization, name string) error {
	return m.RemoveWireGuardPeerFunc(ctx, org, name)
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

func (m *Client) UpdateRelease(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error) {
	return m.UpdateReleaseFunc(ctx, input)
}

func (m *Client) UnsetSecrets(ctx context.Context, appName string, keys []string) (*fly.Release, error) {
	return m.UnsetSecretsFunc(ctx, appName, keys)
}

func (m *Client) ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error) {
	return m.ValidateWireGuardPeersFunc(ctx, peerIPs)
}
