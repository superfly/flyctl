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
	AllocateEgressIPAddress(ctx context.Context, appName string, machineId string) (net.IP, net.IP, error)
	AppNameAvailable(ctx context.Context, appName string) (bool, error)
	AttachPostgresCluster(ctx context.Context, input fly.AttachPostgresClusterInput) (*fly.AttachPostgresClusterPayload, error)
	Authenticated() bool
	CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error)
	ClosestWireguardGatewayRegion(ctx context.Context) (*fly.Region, error)
	CreateDoctorUrl(ctx context.Context) (putUrl string, err error)
	CreateOrganization(ctx context.Context, organizationname string) (*fly.Organization, error)
	CreateOrganizationInvite(ctx context.Context, id, email string) (*fly.Invitation, error)
	CreateWireGuardPeer(ctx context.Context, orgID string, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error)
	DeleteOrganization(ctx context.Context, id string) (deletedid string, err error)
	DeleteOrganizationMembership(ctx context.Context, orgId, userId string) (string, string, error)
	DetachPostgresCluster(ctx context.Context, input fly.DetachPostgresClusterInput) error
	EnablePostgresConsul(ctx context.Context, appName string) (*fly.PostgresEnableConsulPayload, error)
	FinishBuild(ctx context.Context, input fly.FinishBuildInput) (*fly.FinishBuildResponse, error)
	GetApp(ctx context.Context, appName string) (*fly.App, error)
	GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error)
	GetAppHostIssues(ctx context.Context, appName string) ([]fly.HostIssue, error)
	GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]fly.LimitedAccessToken, error)
	GetAppLogs(ctx context.Context, appName, token, region, instanceID string) (entries []fly.LogEntry, nextToken string, err error)
	GetAppNameFromVolume(ctx context.Context, volID string) (*string, error)
	GetAppReleasesMachines(ctx context.Context, appName, status string, limit int) ([]fly.Release, error)
	GetApps(ctx context.Context, role *string) ([]fly.App, error)
	GetCurrentUser(ctx context.Context) (*fly.User, error)
	GetDetailedOrganizationBySlug(ctx context.Context, slug string) (*fly.OrganizationDetails, error)
	GetEgressIPAddresses(ctx context.Context, appName string) (map[string][]fly.EgressIPAddress, error)
	GetLatestImageDetails(ctx context.Context, image string, flyVersion string) (*fly.ImageVersion, error)
	GetLatestImageTag(ctx context.Context, repository string, snapshotId *string) (string, error)
	GetLoggedCertificates(ctx context.Context, slug string) ([]fly.LoggedCertificate, error)
	GetMachine(ctx context.Context, machineId string) (*fly.GqlMachine, error)
	GetOrgLimitedAccessTokens(ctx context.Context, orgSlug string) ([]fly.LimitedAccessToken, error)
	GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error)
	GetAllowedReplaySourceOrgSlugs(ctx context.Context, slug string) ([]string, error)
	AddAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, sourceOrgSlugs []string) (*fly.Organization, error)
	RemoveAllowedReplaySourceOrgs(ctx context.Context, orgSlug string, orgSlugsToRemove []string) (*fly.Organization, error)
	GetAllowAllCrossNetworkReplays(ctx context.Context, slug string) (bool, error)
	SetAllowAllCrossNetworkReplays(ctx context.Context, orgSlug string, allow bool) (*fly.Organization, error)
	GetWireGuardPeers(ctx context.Context, slug string) ([]*fly.WireGuardPeer, error)
	GenqClient() genq.Client
	IssueSSHCertificate(ctx context.Context, orgID string, principals []string, appNames []string, valid_hours *int, publicKey ed25519.PublicKey) (*fly.IssuedCertificate, error)
	ListPostgresClusterAttachments(ctx context.Context, appName, postgresAppName string) ([]*fly.PostgresClusterAttachment, error)
	Logger() fly.Logger
	MoveApp(ctx context.Context, appName string, orgID string) (*fly.App, error)
	NewRequest(q string) *graphql.Request
	PlatformRegions(ctx context.Context) ([]fly.Region, *fly.Region, error)
	ReleaseEgressIPAddress(ctx context.Context, appName string, machineID string) (net.IP, net.IP, error)
	RemoveWireGuardPeer(ctx context.Context, orgID string, name string) error
	ResolveImageForApp(ctx context.Context, appName, imageRef string) (*fly.Image, error)
	RevokeLimitedAccessToken(ctx context.Context, id string) error
	Run(req *graphql.Request) (fly.Query, error)
	RunWithContext(ctx context.Context, req *graphql.Request) (fly.Query, error)
	SetGenqClient(client genq.Client)
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
