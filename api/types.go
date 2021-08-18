package api

import (
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
	AppStatus            AppStatus
	AppCertsCompact      AppCertsCompact
	CurrentUser          User
	PersonalOrganization Organization
	Organizations        struct {
		Nodes []Organization
	}

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

	// aliases & nodes

	TemplateDeploymentNode *TemplateDeployment
	ReleaseCommandNode     *ReleaseCommand

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

	CreateSignedUrl SignedUrls

	StartBuild struct {
		Build Build
	}

	AddCertificate struct {
		Certificate *AppCertificate
		Check       *HostnameCheck
	}

	DeleteCertificate DeleteCertificatePayload

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
		App    App
		VMSize *VMSize
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
		App App
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

	CreateOrganizationInvitation CreateOrganizationInvitation

	ValidateWireGuardPeers struct {
		InvalidPeerIPs []string
	}

	Machines struct {
		Nodes []*Machine
	}
	LaunchMachine struct {
		Machine *Machine
		App     *App
	}
	StopMachine struct {
		Machine *Machine
	}
}

type CreatedWireGuardPeer struct {
	Peerip     string `json:"peerip"`
	Endpointip string `json:"endpointip"`
	Pubkey     string `json:"pubkey"`
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

type MachineConfig map[string]interface{}

func DefinitionPtr(in map[string]interface{}) *Definition {
	x := Definition(in)
	return &x
}

type App struct {
	ID             string
	Name           string
	State          string
	Status         string
	Deployed       bool
	Hostname       string
	AppURL         string
	Version        int
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
	IPAddress *IPAddress
	Builds    struct {
		Nodes []Build
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
	HealthChecks    *struct {
		Nodes []CheckState
	}
	PostgresAppRole *struct {
		Databases *[]PostgresClusterDatabase
		Users     *[]PostgresClusterUser
	}
	Image *Image
}

type TaskGroupCount struct {
	Name  string
	Count int
}

type Volume struct {
	ID                 string `json:"id"`
	App                string
	Name               string
	SizeGb             int
	Region             string
	Encrypted          bool
	CreatedAt          time.Time
	AttachedAllocation *AllocationStatus
}

type CreateVolumeInput struct {
	AppID     string `json:"appId"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	SizeGb    int    `json:"sizeGb"`
	Encrypted bool   `json:"encrypted"`
}

type CreateVolumePayload struct {
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
	ID           string
	Name         string
	Status       string
	Deployed     bool
	Hostname     string
	AppURL       string
	Version      int
	Release      *Release
	Organization Organization
	IPAddresses  struct {
		Nodes []IPAddress
	}
	Services []Service
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
	ID   string
	Name string
	Slug string
	Type string

	Domains struct {
		Nodes *[]*Domain
		Edges *[]*struct {
			Cursor *string
			Node   *Domain
		}
	}

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

type OrganizationDetails struct {
	ID         string
	Name       string
	Slug       string
	Type       string
	ViewerRole string
	Apps       struct {
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
	Runtime         string  `json:"runtime"`
	Name            string  `json:"name"`
	PreferredRegion *string `json:"preferredRegion,omitempty"`
	Network         *string `json:"network,omitempty"`
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
	CreatedAt          time.Time
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

type SignedUrls struct {
	GetUrl string
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

type DeployImageInput struct {
	AppID      string      `json:"appId"`
	Image      string      `json:"image"`
	Services   *[]Service  `json:"services"`
	Definition *Definition `json:"definition"`
	Strategy   *string     `json:"strategy"`
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
	Type              string       `json:"type"`
	Interval          *uint64      `json:"interval"`
	Timeout           *uint64      `json:"timeout"`
	HTTPMethod        *string      `json:"httpMethod"`
	HTTPPath          *string      `json:"httpPath"`
	HTTPProtocol      *string      `json:"httpProtocol"`
	HTTPSkipTLSVerify *bool        `json:"httpTlsSkipVerify"`
	HTTPHeaders       []HTTPHeader `json:"httpHeaders"`
}

type HTTPHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type AllocateIPAddressInput struct {
	AppID string `json:"appId"`
	Type  string `json:"type"`
}

type ReleaseIPAddressInput struct {
	IPAddressID string `json:"ipAddressId"`
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
	Name               string
	CPUCores           float32
	MemoryGB           float32
	MemoryMB           int
	PriceMonth         float32
	PriceSecond        float32
	MemoryIncrementsMB []int
}

type SetVMSizeInput struct {
	AppID    string `json:"appId"`
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

type StartBuildInput struct {
	AppID      string          `json:"appId"`
	SourceURL  string          `json:"sourceUrl"`
	SourceType string          `json:"sourceType"`
	BuildType  *string         `json:"buildType"`
	BuildArgs  []BuildArgInput `json:"buildArgs"`
}

type BuildArgInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ConfigureRegionsInput struct {
	AppID         string   `json:"appId"`
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
	ID     string
	Pubkey string
	Region string
	Name   string
	Peerip string
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
	ImageRef       *string `json:"imageRef,omitempty"`
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

type AttachPostgresClusterInput struct {
	AppID                string  `json:"appId"`
	PostgresClusterAppID string  `json:"postgresClusterAppId"`
	DatabaseName         *string `json:"databaseName,omitempty"`
	VariableName         *string `json:"variableName,omitempty"`
}

type AttachPostgresClusterPayload struct {
	App                     App
	PostgresClusterApp      App
	ConnectionString        string
	EnvironmentVariableName string
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

type Image struct {
	ID             string
	Digest         string
	Ref            string
	CompressedSize uint64
}

type ReleaseCommand struct {
	ID         string
	Command    string
	Status     string
	ExitCode   *int
	InstanceID *string
	InProgress bool
	Succeeded  bool
	Failed     bool
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

type LaunchMachineInput struct {
	AppID   string         `json:"appId,omitempty"`
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	OrgSlug string         `json:"organizationId,omitempty"`
	Region  string         `json:"region,omitempty"`
	Config  *MachineConfig `json:"config"`
}

type Machine struct {
	ID     string
	Name   string
	State  string
	Region string
	Config MachineConfig

	App *App

	CreatedAt time.Time
}

type StopMachineInput struct {
	AppID           string `json:"appId,omitempty"`
	ID              string `json:"id"`
	Signal          string `json:"signal,omitempty"`
	KillTimeoutSecs int    `json:"kill_timeout_secs,omitempty"`
}
