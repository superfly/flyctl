package api

import "time"

type Query struct {
	Apps struct {
		Nodes []App
	}
	App           App
	CurrentUser   User
	Organizations struct {
		Nodes []Organization
	}

	Build Build

	Platform struct {
		Regions []Region
		VMSizes []VMSize
	}

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
		Release Release
	}

	OptimizeImage struct {
		Status string
	}

	CreateSignedUrl SignedUrls

	StartBuild struct {
		Build Build
	}

	AddCertificate struct {
		Certificate AppCertificate
	}

	DeleteCertificate DeleteCertificatePayload
	CheckCertificate  struct {
		App         *App
		Certificate *AppCertificate
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

	ConfigureRegions struct {
		App     App
		Regions []Region
	}

	ResumeApp struct {
		App App
	}

	PauseApp struct {
		App App
	}

	RestartApp struct {
		App App
	}
}

type Definition map[string]interface{}

type App struct {
	ID             string
	Name           string
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
	Services         []Service
	Config           AppConfig
	ParseConfig      AppConfig
	Allocations      []*AllocationStatus
	Allocation       *AllocationStatus
	DeploymentStatus *DeploymentStatus
	Autoscaling      *AutoscalingConfig
	VMSize           VMSize
	Regions          *[]Region
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
	OrganizationID string `json:"organizationId"`
	Runtime        string `json:"runtime"`
	Name           string `json:"name"`
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
	Deployment         DeploymentStatus
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
	Issued                    struct {
		Nodes []struct {
			ExpiresAt time.Time
			Type      string
		}
	}
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
	Protocol        string        `json:"protocol"`
	InternalPort    int           `json:"internalPort"`
	Ports           []PortHandler `json:"ports"`
	Checks          []Check       `json:"checks"`
	SoftConcurrency int           `json:"softConcurrency"`
	HardConcurrency int           `json:"hardConcurrency"`
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
	RecentLogs         []LogEntry
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
}

type Region struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Latitude  float32 `json:"latitude,omitempty"`
	Longitude float32 `json:"longitude,omitempty"`
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
	MemoryGB    float32
	MemoryMB    int
	PriceMonth  float32
	PriceSecond float32
}

type SetVMSizeInput struct {
	AppID    string `json:"appId"`
	SizeName string `json:"sizeName"`
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
	AppID        string   `json:"appId"`
	AllowRegions []string `json:"allowRegions"`
	DenyRegions  []string `json:"denyRegions"`
}
