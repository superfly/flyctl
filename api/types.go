package api

import (
	"fmt"
	"syscall"
	"time"
)

// Query - Master query which encapsulates all possible returned structures
type Query struct {
	Errors Errors

	Apps struct {
		Nodes []App
	}
	App                  App
	AppCompact           AppCompact
	AppInfo              AppInfo
	AppBasic             AppBasic
	AppStatus            AppStatus
	AppMonitoring        AppMonitoring
	AppPostgres          AppPostgres
	AppCertsCompact      AppCertsCompact
	CurrentUser          User
	PersonalOrganization Organization
	GqlMachine           GqlMachine
	Organizations        struct {
		Nodes []Organization
	}

	AddOns struct {
		Nodes []AddOn
	}

	AddOn *AddOn

	Organization *Organization
	// PersonalOrganizations PersonalOrganizations
	OrganizationDetails OrganizationDetails
	Build               Build
	Volume              Volume
	Domain              *Domain

	Node  interface{}
	Nodes []interface{}

	Platform struct {
		RequestRegion string
		Regions       []Region
		VMSizes       []VMSize
	}

	NearestRegion *Region

	LatestImageTag     string
	LatestImageDetails ImageVersion
	// aliases & nodes

	TemplateDeploymentNode *TemplateDeployment
	ReleaseCommandNode     *ReleaseCommand

	ValidateConfig AppConfig

	// hack to let us alias node to a type
	// DNSZone *DNSZone

	// mutations
	CreateApp struct {
		App App
	}

	SetSecrets struct {
		Release Release
	}

	UnsetSecrets struct {
		Release Release
	}

	DeployImage struct {
		Release        Release
		ReleaseCommand *ReleaseCommand
	}

	EnsureRemoteBuilder *struct {
		App     *App
		URL     string
		Release Release
	}

	EnsureMachineRemoteBuilder *struct {
		App     *App
		Machine *GqlMachine
	}

	CreateDoctorUrl SignedUrl

	AddCertificate struct {
		Certificate *AppCertificate
		Check       *HostnameCheck
	}

	DeleteCertificate DeleteCertificatePayload

	DeleteAddOn DeleteAddOnPayload

	CheckCertificate struct {
		App         *App
		Certificate *AppCertificate
		Check       *HostnameCheck
	}

	AllocateIPAddress struct {
		App       App
		IPAddress IPAddress
	}
	ReleaseIPAddress struct {
		App App
	}
	ScaleApp struct {
		App       App
		Placement []RegionPlacement
		Delta     []ScaleRegionChange
	}

	UpdateAutoscaleConfig struct {
		App App
	}

	SetVMSize struct {
		App          App
		VMSize       *VMSize
		ProcessGroup *ProcessGroup
	}

	SetVMCount struct {
		App             App
		TaskGroupCounts []TaskGroupCount
		Warnings        []string
	}

	ConfigureRegions struct {
		App           App
		Regions       []Region
		BackupRegions []Region
	}

	ResumeApp struct {
		App AppCompact
	}

	SuspendApp struct {
		App App
	}

	RestartApp struct {
		App App
	}

	CreateDomain struct {
		Domain *Domain
	}
	CreateAndRegisterDomain struct {
		Domain *Domain
	}

	CheckDomain *CheckDomainResult

	ExportDnsZone struct {
		Contents string
	}

	ImportDnsZone struct {
		Warnings []ImportDnsWarning
		Changes  []ImportDnsChange
	}
	CreateOrganization CreateOrganizationPayload
	DeleteOrganization DeleteOrganizationPayload

	CreateVolume CreateVolumePayload
	DeleteVolume DeleteVolumePayload
	ExtendVolume ExtendVolumePayload

	AddWireGuardPeer              CreatedWireGuardPeer
	EstablishSSHKey               SSHCertificate
	IssueCertificate              IssuedCertificate
	CreateDelegatedWireGuardToken DelegatedWireGuardToken
	DeleteDelegatedWireGuardToken DelegatedWireGuardToken

	RemoveWireGuardPeer struct {
		Organization Organization
	}

	SetSlackHandler *struct {
		Handler *HealthCheckHandler
	}

	SetPagerdutyHandler *struct {
		Handler *HealthCheckHandler
	}

	CreatePostgresCluster *CreatePostgresClusterPayload

	AttachPostgresCluster *AttachPostgresClusterPayload

	EnablePostgresConsul *PostgresEnableConsulPayload

	CreateOrganizationInvitation CreateOrganizationInvitation

	ValidateWireGuardPeers struct {
		InvalidPeerIPs []string
	}

	PostgresAttachments struct {
		Nodes []*PostgresClusterAttachment
	}

	DeleteOrganizationMembership *DeleteOrganizationMembershipPayload

	UpdateRemoteBuilder struct {
		Organization Organization
	}

	ProvisionAddOn ProvisionAddOnPayload
}

type CreatedWireGuardPeer struct {
	Peerip     string `json:"peerip"`
	Endpointip string `json:"endpointip"`
	Pubkey     string `json:"pubkey"`
}

type DeleteOrganizationMembershipPayload struct {
	Organization *Organization
	User         *User
}

type DelegatedWireGuardToken struct {
	Token string
}

type DelegatedWireGuardTokenHandle /* whatever */ struct {
	Name string
}

type SSHCertificate struct {
	Certificate string
}

type IssuedCertificate struct {
	Certificate string
	Key         string
}

type Definition map[string]interface{}

type MachineInit struct {
	Exec       []string `json:"exec"`
	Entrypoint []string `json:"entrypoint"`
	Cmd        []string `json:"cmd"`
	Tty        bool     `json:"tty"`
}

func DefinitionPtr(in map[string]interface{}) *Definition {
	x := Definition(in)
	return &x
}

type ImageVersion struct {
	Registry   string
	Repository string
	Tag        string
	Version    string
	Digest     string
}

func (img *ImageVersion) FullImageRef() string {
	return fmt.Sprintf("%s/%s:%s", img.Registry, img.Repository, img.Tag)
}

func (img *ImageVersion) ImageRef() string {
	return fmt.Sprintf("%s:%s", img.Repository, img.Tag)
}

type App struct {
	ID        string
	Name      string
	State     string
	Status    string
	Deployed  bool
	Hostname  string
	AppURL    string
	Version   int
	NetworkID int

	Release        *Release
	Organization   Organization
	Secrets        []Secret
	CurrentRelease *Release
	Releases       struct {
		Nodes []Release
	}
	IPAddresses struct {
		Nodes []IPAddress
	}
	SharedIPAddress string
	IPAddress       *IPAddress
	Builds          struct {
		Nodes []Build
	}
	SourceBuilds struct {
		Nodes []SourceBuild
	}
	Changes struct {
		Nodes []AppChange
	}
	Certificates struct {
		Nodes []AppCertificate
	}
	Certificate      AppCertificate
	Config           AppConfig
	ParseConfig      AppConfig
	Allocations      []*AllocationStatus
	Allocation       *AllocationStatus
	DeploymentStatus *DeploymentStatus
	Autoscaling      *AutoscalingConfig
	VMSize           VMSize
	Regions          *[]Region
	BackupRegions    *[]Region
	Volumes          struct {
		Nodes []Volume
	}
	TaskGroupCounts []TaskGroupCount
	ProcessGroups   []ProcessGroup
	HealthChecks    *struct {
		Nodes []CheckState
	}
	PostgresAppRole *struct {
		Name      string
		Databases *[]PostgresClusterDatabase
		Users     *[]PostgresClusterUser
	}
	Image *Image

	ImageUpgradeAvailable       bool
	ImageVersionTrackingEnabled bool
	ImageDetails                ImageVersion
	LatestImageDetails          ImageVersion

	PlatformVersion string
}

type TaskGroupCount struct {
	Name  string
	Count int
}

type Snapshot struct {
	ID        string `json:"id"`
	Digest    string
	Size      string
	CreatedAt time.Time
}

type Volume struct {
	ID  string `json:"id"`
	App struct {
		Name            string
		PlatformVersion string
	}
	Name      string
	SizeGb    int
	Snapshots struct {
		Nodes []Snapshot
	}
	State              string
	Region             string
	Encrypted          bool
	CreatedAt          time.Time
	AttachedAllocation *AllocationStatus
	AttachedMachine    *GqlMachine
	Host               struct {
		ID string
	}
}

func (v *Volume) IsAttached() bool {
	return v.AttachedAllocation != nil || v.AttachedMachine != nil
}

type ProvisionAddOnInput struct {
	OrganizationId string `json:"organizationId"`
	Region         string `json:"region"`
	Type           string `json:"type"`
}

type ProvisionAddOnPayload struct {
	Service AddOn
}

type CreateVolumeInput struct {
	AppID             string  `json:"appId"`
	Name              string  `json:"name"`
	Region            string  `json:"region"`
	SizeGb            int     `json:"sizeGb"`
	Encrypted         bool    `json:"encrypted"`
	SnapshotID        *string `json:"snapshotId,omitempty"`
	RequireUniqueZone bool    `json:"requireUniqueZone"`
}

type ExtendVolumeInput struct {
	VolumeID string `json:"volumeId"`
	SizeGb   int    `json:"sizeGb"`
}

type CreateVolumePayload struct {
	App    App
	Volume Volume
}

type ExtendVolumePayload struct {
	App    App
	Volume Volume
}

type DeleteVolumeInput struct {
	VolumeID string `json:"volumeId"`
}

type DeleteVolumePayload struct {
	App App
}

type AppCertsCompact struct {
	Certificates struct {
		Nodes []AppCertificateCompact
	}
}

type AppCertificateCompact struct {
	CreatedAt    time.Time
	Hostname     string
	ClientStatus string
}

type AppCompact struct {
	ID              string
	Name            string
	Status          string
	Deployed        bool
	Hostname        string
	AppURL          string
	Organization    *OrganizationBasic
	PlatformVersion string
	PostgresAppRole *struct {
		Name string
	}
	ImageDetails ImageVersion
}

func (app *AppCompact) IsPostgresApp() bool {
	// check app.PostgresAppRole.Name == "postgres_cluster"
	return app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster"
}

type AppInfo struct {
	ID              string
	Name            string
	Status          string
	Deployed        bool
	Hostname        string
	Version         int
	PlatformVersion string
	Organization    *OrganizationBasic
	IPAddresses     struct {
		Nodes []IPAddress
	}
	Services []Service
}

type AppBasic struct {
	ID              string
	Name            string
	PlatformVersion string
	Organization    *OrganizationBasic
}

type AppMonitoring struct {
	ID             string
	CurrentRelease *Release
}

type AppPostgres struct {
	ID              string
	Name            string
	Organization    *OrganizationBasic
	ImageDetails    ImageVersion
	PostgresAppRole *struct {
		Name      string
		Databases *[]PostgresClusterDatabase
		Users     *[]PostgresClusterUser
	}
	PlatformVersion string
	Services        []Service
}

func (app *AppPostgres) IsPostgresApp() bool {
	// check app.PostgresAppRole.Name == "postgres_cluster"
	return app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster"
}

type AppStatus struct {
	ID               string
	Name             string
	Deployed         bool
	Status           string
	Hostname         string
	Version          int
	AppURL           string
	Organization     Organization
	DeploymentStatus *DeploymentStatus
	Allocations      []*AllocationStatus
}

type AppConfig struct {
	Definition Definition
	Services   []Service
	Valid      bool
	Errors     []string
}
type Organization struct {
	ID                 string
	InternalNumericID  string
	Name               string
	RemoteBuilderImage string
	RemoteBuilderApp   *App
	Slug               string
	Type               string
	Domains            struct {
		Nodes *[]*Domain
		Edges *[]*struct {
			Cursor *string
			Node   *Domain
		}
	}

	WireGuardPeer *WireGuardPeer

	WireGuardPeers struct {
		Nodes *[]*WireGuardPeer
		Edges *[]*struct {
			Cursor *string
			Node   *WireGuardPeer
		}
	}

	DelegatedWireGuardTokens struct {
		Nodes *[]*DelegatedWireGuardTokenHandle
		Edges *[]*struct {
			Cursor *string
			Node   *DelegatedWireGuardTokenHandle
		}
	}

	HealthCheckHandlers *struct {
		Nodes []HealthCheckHandler
	}

	HealthChecks *struct {
		Nodes []HealthCheck
	}

	LoggedCertificates *struct {
		Nodes []LoggedCertificate
	}
}

func (o *Organization) GetID() string {
	return o.ID
}

func (o *Organization) GetSlug() string {
	return o.Slug
}

type OrganizationBasic struct {
	ID   string
	Slug string
}

func (o *OrganizationBasic) GetID() string {
	return o.ID
}

func (o *OrganizationBasic) GetSlug() string {
	return o.Slug
}

type OrganizationImpl interface {
	GetID() string
	GetSlug() string
}

type OrganizationDetails struct {
	ID                 string
	InternalNumericID  string
	Name               string
	RemoteBuilderImage string
	RemoteBuilderApp   *App
	Slug               string
	Type               string
	ViewerRole         string
	Apps               struct {
		Nodes []App
	}
	Members struct {
		Edges []OrganizationMembershipEdge
	}
}

type OrganizationMembershipEdge struct {
	Cursor   string
	Node     User
	Role     string
	JoinedAt time.Time
}

type Billable struct {
	Category string
	Product  string
	Time     time.Time
	Quantity float64
	App      App
}

type DNSRecords struct {
	ID         string
	Name       string
	Ttl        int
	Values     []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Fqdn       string
	IsApex     bool
	IsSystem   bool
	IsWildcard bool
	Domain     *Domain
}

type IPAddress struct {
	ID        string
	Address   string
	Type      string
	Region    string
	CreatedAt time.Time
}

type User struct {
	ID    string
	Name  string
	Email string
}

type Secret struct {
	Name      string
	Digest    string
	CreatedAt time.Time
}

type SetSecretsInput struct {
	AppID   string                  `json:"appId"`
	Secrets []SetSecretsInputSecret `json:"secrets"`
}

type SetSecretsInputSecret struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UnsetSecretsInput struct {
	AppID string   `json:"appId"`
	Keys  []string `json:"keys"`
}

type CreateAppInput struct {
	OrganizationID  string  `json:"organizationId"`
	Name            string  `json:"name"`
	PreferredRegion *string `json:"preferredRegion,omitempty"`
	Network         *string `json:"network,omitempty"`
	AppRoleID       string  `json:"appRoleId,omitempty"`
	Machines        bool    `json:"machines,omitempty"`
}

type LogEntry struct {
	Timestamp string
	Message   string
	Level     string
	Instance  string
	Region    string
	Meta      struct {
		Instance string
		Region   string
		Event    struct {
			Provider string
		}
		HTTP struct {
			Request struct {
				ID      string
				Method  string
				Version string
			}
			Response struct {
				StatusCode int `json:"status_code"`
			}
		}
		Error struct {
			Code    int
			Message string
		}
		URL struct {
			Full string
		}
	}
}

type Release struct {
	ID                 string
	Version            int
	Stable             bool
	InProgress         bool
	Reason             string
	Description        string
	Status             string
	DeploymentStrategy string
	User               User
	EvaluationID       string
	CreatedAt          time.Time
	ImageRef           string
}

type Build struct {
	ID         string
	InProgress bool
	Status     string
	User       User
	Logs       string
	Image      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SourceBuild struct {
	ID        string
	Status    string
	User      User
	Logs      string
	Image     string
	AppName   string
	MachineId string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SignedUrl struct {
	PutUrl string
}

type AppChange struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Actor     struct {
		Type string
	}
	Status      string
	Description string
	Reason      string
	User        User
}

type DeploymentStatus struct {
	ID             string
	Status         string
	Description    string
	InProgress     bool
	Successful     bool
	CreatedAt      time.Time
	Allocations    []*AllocationStatus
	Version        int
	DesiredCount   int
	PlacedCount    int
	HealthyCount   int
	UnhealthyCount int
}

type AppCertificate struct {
	ID                        string
	AcmeDNSConfigured         bool
	AcmeALPNConfigured        bool
	Configured                bool
	CertificateAuthority      string
	CreatedAt                 time.Time
	DNSProvider               string
	DNSValidationInstructions string
	DNSValidationHostname     string
	DNSValidationTarget       string
	Hostname                  string
	Source                    string
	ClientStatus              string
	IsApex                    bool
	IsWildcard                bool
	Issued                    struct {
		Nodes []struct {
			ExpiresAt time.Time
			Type      string
		}
	}
}

type CreateOrganizationPayload struct {
	Organization Organization
}

type DeleteOrganizationPayload struct {
	DeletedOrganizationId string
}

type HostnameCheck struct {
	ARecords              []string `json:"aRecords"`
	AAAARecords           []string `json:"aaaaRecords"`
	CNAMERecords          []string `json:"cnameRecords"`
	SOA                   string   `json:"soa"`
	DNSProvider           string   `json:"dnsProvider"`
	DNSVerificationRecord string   `json:"dnsVerificationRecord"`
	ResolvedAddresses     []string `json:"resolvedAddresses"`
}

type DeleteCertificatePayload struct {
	App         App
	Certificate AppCertificate
}

type DeleteAddOnPayload struct {
	ID string
}

type DeployImageInput struct {
	AppID      string      `json:"appId"`
	Image      string      `json:"image"`
	Services   *[]Service  `json:"services"`
	Definition *Definition `json:"definition"`
	Strategy   *string     `json:"strategy"`
}

type Signal struct {
	syscall.Signal
}

type Service struct {
	Description     string        `json:"description"`
	Protocol        string        `json:"protocol,omitempty"`
	InternalPort    int           `json:"internalPort,omitempty"`
	Ports           []PortHandler `json:"ports,omitempty"`
	Checks          []Check       `json:"checks,omitempty"`
	SoftConcurrency int           `json:"softConcurrency,omitempty"`
	HardConcurrency int           `json:"hardConcurrency,omitempty"`
}

type PortHandler struct {
	Port     int      `json:"port"`
	Handlers []string `json:"handlers"`
}

type Check struct {
	Type              string       `json:"type,omitempty"`
	Interval          *uint64      `json:"interval,omitempty"`
	Timeout           *uint64      `json:"timeout,omitempty"`
	GracePeriod       *uint64      `json:"grace_period,omitempty"`
	RestartLimit      *uint64      `json:"restart_limit,omitempty"`
	HTTPMethod        *string      `json:"httpMethod,omitempty"`
	HTTPPath          *string      `json:"httpPath,omitempty"`
	HTTPProtocol      *string      `json:"httpProtocol,omitempty"`
	HTTPSkipTLSVerify *bool        `json:"httpTlsSkipVerify,omitempty"`
	HTTPHeaders       []HTTPHeader `json:"httpHeaders,omitempty"`
}

type HTTPHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type AllocateIPAddressInput struct {
	AppID          string `json:"appId"`
	Type           string `json:"type"`
	Region         string `json:"region"`
	OrganizationID string `json:"organizationId,omitempty"`
	Network        string `json:"network,omitempty"`
}

type ReleaseIPAddressInput struct {
	AppID       *string `json:"appId"`
	IPAddressID *string `json:"ipAddressId"`
	IP          *string `json:"ip"`
}

type ScaleAppInput struct {
	AppID   string             `json:"appId"`
	Regions []ScaleRegionInput `json:"regions"`
}

type ScaleRegionInput struct {
	Region string `json:"region"`
	Count  int    `json:"count"`
}

type ScaleRegionChange struct {
	Region    string
	FromCount int
	ToCount   int
}

type RegionPlacement struct {
	Region string
	Count  int
}

type AllocationStatus struct {
	ID                 string
	IDShort            string
	Version            int
	TaskName           string
	Region             string
	Status             string
	DesiredStatus      string
	Healthy            bool
	Canary             bool
	Failed             bool
	Restarts           int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Checks             []CheckState
	Events             []AllocationEvent
	LatestVersion      bool
	PassingCheckCount  int
	WarningCheckCount  int
	CriticalCheckCount int
	Transitioning      bool
	PrivateIP          string
	RecentLogs         []LogEntry
	AttachedVolumes    struct {
		Nodes []Volume
	}
}

type AllocationEvent struct {
	Timestamp time.Time
	Type      string
	Message   string
}

type CheckState struct {
	Name        string
	Status      string
	Output      string
	ServiceName string
	Allocation  *AllocationStatus
	Type        string
	UpdatedAt   time.Time
}

type Region struct {
	Code             string
	Name             string
	Latitude         float32
	Longitude        float32
	GatewayAvailable bool
}

type AutoscalingConfig struct {
	BalanceRegions bool
	Enabled        bool
	MaxCount       int
	MinCount       int
	Regions        []AutoscalingRegionConfig
}

type AutoscalingRegionConfig struct {
	Code     string
	MinCount int
	Weight   int
}

type UpdateAutoscaleConfigInput struct {
	AppID          string                       `json:"appId"`
	Enabled        *bool                        `json:"enabled"`
	MinCount       *int                         `json:"minCount"`
	MaxCount       *int                         `json:"maxCount"`
	BalanceRegions *bool                        `json:"balanceRegions"`
	ResetRegions   *bool                        `json:"resetRegions"`
	Regions        []AutoscaleRegionConfigInput `json:"regions"`
}

type AutoscaleRegionConfigInput struct {
	Code     string `json:"code"`
	MinCount *int   `json:"minCount"`
	Weight   *int   `json:"weight"`
	Reset    *bool  `json:"reset"`
}

type VMSize struct {
	Name        string
	CPUCores    float32
	CPUClass    string
	MemoryGB    float32
	MemoryMB    int
	PriceMonth  float32
	PriceSecond float32
	// MemoryIncrementsMB []int
}

type ProcessGroup struct {
	Name         string
	Regions      []string
	MaxPerRegion int
	VMSize       *VMSize
}

type SetVMSizeInput struct {
	AppID    string `json:"appId"`
	Group    string `json:"group"`
	SizeName string `json:"sizeName"`
	MemoryMb int64  `json:"memoryMb"`
}

type SetVMCountInput struct {
	AppID       string         `json:"appId"`
	GroupCounts []VMCountInput `json:"groupCounts"`
}

type VMCountInput struct {
	Group        string `json:"group"`
	Count        int    `json:"count"`
	MaxPerRegion *int   `json:"maxPerRegion"`
}

type StartSourceBuildInput struct {
	AppID string `json:"appId"`
}

type BuildArgInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ConfigureRegionsInput struct {
	AppID         string   `json:"appId"`
	Group         string   `json:"group"`
	AllowRegions  []string `json:"allowRegions"`
	DenyRegions   []string `json:"denyRegions"`
	BackupRegions []string `json:"backupRegions"`
}

type Errors []Error

type Error struct {
	Message    string
	Path       []string
	Extensions Extensions
}

type Extensions struct {
	Code        string
	ServiceName string
	Query       string
	Variables   map[string]string
}

type Domain struct {
	ID                   string
	Name                 string
	CreatedAt            time.Time
	Organization         *Organization
	AutoRenew            *bool
	DelegatedNameservers *[]string
	ZoneNameservers      *[]string
	DnsStatus            *string
	RegistrationStatus   *string
	ExpiresAt            time.Time
	DnsRecords           *struct {
		Nodes *[]*DNSRecord
	}
}

type CheckDomainResult struct {
	DomainName            string
	TLD                   string
	RegistrationSupported bool
	RegistrationAvailable bool
	RegistrationPrice     int
	RegistrationPeriod    int
	TransferAvailable     bool
	DnsAvailable          bool
}

type DNSRecord struct {
	ID         string
	Name       string
	FQDN       string
	IsApex     bool
	IsWildcard bool
	IsSystem   bool
	TTL        int
	Type       string
	RData      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ImportDnsChange struct {
	Action  string
	OldText string
	NewText string
}

type ImportDnsWarning struct {
	Action     string
	Attributes struct {
		Name  string
		Type  string
		TTL   int
		Rdata string
	}
	Message string
}

type WireGuardPeer struct {
	ID            string
	Pubkey        string
	Region        string
	Name          string
	Peerip        string
	GatewayStatus *WireGuardPeerStatus
}

type WireGuardPeerStatus struct {
	Endpoint       string
	LastHandshake  string
	SinceHandshake string
	Rx             int64
	Tx             int64
	Added          string
	SinceAdded     string
	Live           bool
	WgError        string
}

type LoggedCertificate struct {
	Root bool
	Cert string
}

type HealthCheck struct {
	Entity      string
	Name        string
	Output      string
	State       string
	LastPassing time.Time
}

type HealthCheckHandler struct {
	Name string
	Type string
}

type SetSlackHandlerInput struct {
	OrganizationID  string  `json:"organizationId"`
	Name            string  `json:"name"`
	SlackWebhookURL string  `json:"slackWebhookUrl"`
	SlackChannel    *string `json:"slackChannel"`
	SlackUsername   *string `json:"slackUsername"`
	SlackIconURL    *string `json:"slackIconUrl"`
}

type SetPagerdutyHandlerInput struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	PagerdutyToken string `json:"pagerdutyToken"`
}

type CreatePostgresClusterInput struct {
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Region         *string `json:"region,omitempty"`
	Password       *string `json:"password,omitempty"`
	VMSize         *string `json:"vmSize,omitempty"`
	VolumeSizeGB   *int    `json:"volumeSizeGb,omitempty"`
	Count          *int    `json:"count,omitempty"`
	ImageRef       *string `json:"imageRef,omitempty"`
	SnapshotID     *string `json:"snapshotId,omitempty"`
}

type CreatePostgresClusterPayload struct {
	App      *App
	Username string
	Password string
}

type TemplateDeployment struct {
	ID     string
	Status string
	Apps   *struct {
		Nodes []App
	}
}

type NomadToMachinesMigrationInput struct {
	AppID string `json:"appId"`
}

type NomadToMachinesMigrationPrepInput struct {
	AppID string `json:"appId"`
}

type AttachPostgresClusterInput struct {
	AppID                string  `json:"appId"`
	PostgresClusterAppID string  `json:"postgresClusterAppId"`
	DatabaseName         *string `json:"databaseName,omitempty"`
	DatabaseUser         *string `json:"databaseUser,omitempty"`
	VariableName         *string `json:"variableName,omitempty"`
	ManualEntry          bool    `json:"manualEntry,omitempty"`
}

type DetachPostgresClusterInput struct {
	AppID                       string `json:"appId"`
	PostgresClusterId           string `json:"postgresClusterAppId"`
	PostgresClusterAttachmentId string `json:"postgresClusterAttachmentId"`
}

type AttachPostgresClusterPayload struct {
	App                     App
	PostgresClusterApp      App
	ConnectionString        string
	EnvironmentVariableName string
}

type PostgresEnableConsulPayload struct {
	ConsulURL string `json:"consulUrl"`
}

type EnsureRemoteBuilderInput struct {
	AppName        *string `json:"appName"`
	OrganizationID *string `json:"organizationId"`
}

type PostgresClusterUser struct {
	Username    string
	IsSuperuser bool
	Databases   []string
}

type PostgresClusterDatabase struct {
	Name  string
	Users []string
}

type PostgresClusterAttachment struct {
	ID                      string
	DatabaseName            string
	DatabaseUser            string
	EnvironmentVariableName string
}

type Image struct {
	ID             string
	Digest         string
	Ref            string
	CompressedSize uint64
}

type ReleaseCommand struct {
	ID           string
	Command      string
	Status       string
	ExitCode     *int
	InstanceID   *string
	InProgress   bool
	Succeeded    bool
	Failed       bool
	EvaluationID string
}

type Invitation struct {
	ID           string
	Email        string
	CreatedAt    time.Time
	Redeemed     bool
	Inviter      *User
	Organization *Organization
}

type CreateOrganizationInvitation struct {
	Invitation Invitation
}

type GqlMachine struct {
	ID     string
	Name   string
	State  string
	Region string
	Config MachineConfig

	App *AppCompact

	IPs struct {
		Nodes []*MachineIP
	}
}

type Condition struct {
	Equal    interface{} `json:"equal,omitempty"`
	NotEqual interface{} `json:"not_equal,omitempty"`
}

type Filters struct {
	AppName      string               `json:"app_name"`
	MachineState []Condition          `json:"machine_state"`
	Meta         map[string]Condition `json:"meta"`
}

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
}
type AddOn struct {
	PublicUrl     string
	Name          string
	ID            string
	PrimaryRegion string
	Organization  *OrganizationBasic
}
