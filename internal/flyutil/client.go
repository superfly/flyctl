package flyutil

import (
	"context"
	"crypto/ed25519"
	"net"

	genq "github.com/Khan/genqlient/graphql"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/graphql"
)

var _ Client = (*fly.Client)(nil)

type Client interface {
	AddCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error)
	AllocateEgressIPAddress(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error)
	AttachPostgresCluster(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error)
	Authenticated() bool
	CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error)
	CheckAppCertificate(ctx context.Context, appName, hostname string) (*fly.AppCertificate, *fly.HostnameCheck, error)
	CheckDomain(ctx context.Context, name string) (*fly.CheckDomainResult, error)
	ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error)
	CreateApp(ctx context.Context, input fly.CreateAppInput) (*fly.App, error)
	CreateBuild(ctx context.Context, input fly.CreateBuildInput) (*fly.CreateBuildResponse, error)
	CreateDelegatedWireGuardToken(ctx context.Context, orgID string, name string) (*fly.DelegatedWireGuardToken, error)
	CreateDoctorUrl(ctx context.Context) (putUrl string, err error)
	CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error)
	CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error)
	CreateRelease(ctx context.Context, input fly.CreateReleaseInput) (*fly.CreateReleaseResponse, error)
	CreateWireGuardPeer(ctx context.Context, orgID string, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error)
	DeleteApp(ctx context.Context, appName string) error
	DeleteCertificate(ctx context.Context, appName, hostname string) (*fly.DeleteCertificatePayload, error)
	DeleteDelegatedWireGuardToken(ctx context.Context, orgID string, name, token *string) error
	DeleteOrganization(ctx context.Context, id string) (deletedid string, err error)
	DeleteOrganizationMembership(ctx context.Context, orgId, userId string) (string, string, error)
	DetachPostgresCluster(ctx context.Context, input fly.DetachPostgresClusterInput) error
	EnablePostgresConsul(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error)
	EnsureRemoteBuilder(ctx context.Context, orgID, appName, region string) (*fly.GqlMachine, *fly.App, error)
	EnsureDepotRemoteBuilder(ctx context.Context, input *fly.EnsureDepotRemoteBuilderInput) (*fly.EnsureDepotRemoteBuilderResponse, error)
	FinishBuild(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error)
	GetApp(ctx context.Context, appName string) (*fly.App, error)
	GetAppRemoteBuilder(ctx context.Context, appName string) (*fly.App, error)
	GetAppBasic(ctx context.Context, appName string) (*fly.AppBasic, error)
	GetAppCertificates(ctx context.Context, appName string) ([]fly.AppCertificateCompact, error)
	GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error)
	GetAppCurrentReleaseMachines(ctx context.Context, appName string) (*fly.Release, error)
	GetAppCNAMETarget(ctx context.Context, appName string) (string, error)
	GetAppHostIssues(ctx context.Context, appName string) ([]fly.HostIssue, error)
	GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error)
	GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error)
	GetAppNameFromVolume(ctx context.Context, volID string) (*string, error)
	GetAppNameStateFromVolume(ctx context.Context, volID string) (*string, *string, error)
	GetAppNetwork(ctx context.Context, appName string) (*string, error)
	GetAppReleasesMachines(ctx context.Context, appName, status string, limit int) ([]fly.Release, error)
	GetApps(ctx context.Context, role *string) ([]fly.App, error)
	GetAppsForOrganization(ctx context.Context, orgID string) ([]fly.App, error)
	GetDeployerAppByOrg(ctx context.Context, orgID string) (*fly.App, error)
	GetCurrentUser(ctx context.Context) (*fly.User, error)
	GetDelegatedWireGuardTokens(ctx context.Context, slug string) ([]*fly.DelegatedWireGuardTokenHandle, error)
	GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error)
	GetEgressIPAddresses(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error)
	GetLatestImageDetails(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error)
	GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error)
	GetLoggedCertificates(ctx context.Context, slug string) ([]fly.LoggedCertificate, error)
	GetMachine(ctx context.Context, machineId string) (*fly.GqlMachine, error)
	GetNearestRegion(ctx context.Context) (*fly.Region, error)
	GetOrganizationBySlug(ctx context.Context, slug string) (*fly.Organization, error)
	GetOrganizationByApp(ctx context.Context, appName string) (*fly.Organization, error)
	GetOrganizationRemoteBuilderBySlug(ctx context.Context, slug string) (*fly.Organization, error)
	GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error)
	GetSnapshotsFromVolume(ctx context.Context, volID string) ([]fly.VolumeSnapshot, error)
	GetWireGuardPeer(ctx context.Context, slug, name string) (*fly.WireGuardPeer, error)
	GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error)
	GenqClient() genq.Client
	IssueSSHCertificate(ctx context.Context, orgID string, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error)
	LatestImage(ctx context.Context, appName string) (string, error)
	ListPostgresClusterAttachments(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error)
	Logger() fly.Logger
	MoveApp(ctx context.Context, appName string, orgID string) (*fly.App, error)
	NewRequest(q string) *graphql.Request
	ReleaseEgressIPAddress(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error)
	RemoveWireGuardPeer(ctx context.Context, orgID string, name string) error
	ResolveImageForApp(ctx context.Context, appName, imageRef string) (*fly.Image, error)
	RevokeLimitedAccessToken(ctx context.Context, id string) error
	Run(req *graphql.Request) (fly.Query, error)
	RunWithContext(ctx context.Context, req *graphql.Request) (fly.Query, error)
	SetGenqClient(client genq.Client)
	UpdateRelease(ctx context.Context, input fly.UpdateReleaseInput) (*fly.UpdateReleaseResponse, error)
	ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error)
}

type contextKey string

const contextKeyClient = contextKey("client")

// NewContextWithClient derives a Context that carries c from ctx.
func NewContextWithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, contextKeyClient, c)
}

// ClientFromContext returns the Client ctx carries.
func ClientFromContext(ctx context.Context) Client {
	c, _ := ctx.Value(contextKeyClient).(Client)
	return c
}
