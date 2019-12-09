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
	Databases struct {
		Nodes []Database
	}

	Build    Build
	Database Database

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

	CreateSignedUrl SignedUrls

	CreateBuild struct {
		Build Build
	}

	CreateDatabase struct {
		Database Database
	}

	DestroyDatabase struct {
		Organization Organization
	}

	AddCertificate struct {
		Certificate AppCertificate
	}

	DeleteCertificate DeleteCertificatePayload

	AllocateIPAddress struct {
		App       App
		IPAddress IPAddress
	}
	ReleaseIPAddress struct {
		App App
	}
}

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
	Tasks          []Task
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
	Certificate AppCertificate
	Services    []Service
}

type Organization struct {
	ID   string
	Name string
	Slug string

	Databases struct {
		Nodes []Database
	}
}

type Task struct {
	ID              string
	Name            string
	Status          string
	ServicesSummary string
	Services        []TaskService
	Allocations     []Allocation
}

type TaskService struct {
	ID           string
	Protocol     string
	Ports        []PortHandler
	InternalPort int
	Description  string
}

type Allocation struct {
	ID            string
	Version       int
	LatestVersion bool
	Status        string
	DesiredStatus string
	Region        string
	CreatedAt     time.Time
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
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SignedUrls struct {
	GetUrl string
	PutUrl string
}

type Database struct {
	ID           string
	BackendID    string
	Key          string
	Name         string
	Engine       string
	CreatedAt    time.Time
	VMURL        string
	PublicURL    string
	Organization Organization
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
	ID          string
	Status      string
	Description string
	InProgress  bool
	Tasks       []TaskDeploymentStatus
	CreatedAt   time.Time
}

type TaskDeploymentStatus struct {
	Name             string
	Promoted         bool
	ProgressDeadline time.Time
	Canaries         int
	Desired          int
	Healthy          int
	Unhealthy        int
	Placed           int
}

type AppCertificate struct {
	ID                        string
	AcmeDNSConfigured         bool
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
	AppID    string     `json:"appId"`
	Image    string     `json:"image"`
	Services *[]Service `json:"services"`
}

// mostly duplicate of TaskService but works with the deployImage mutation.
// clean up when we figure out groups/tasks/services
type Service struct {
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
