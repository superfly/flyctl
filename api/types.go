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
		Deployment Deployment
		Release    Release
	}

	UnsetSecrets struct {
		Deployment Deployment
		Release    Release
	}

	DeployImage struct {
		Deployment Deployment
		Release    Release
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
}

type App struct {
	ID           string
	Name         string
	Status       string
	Deployed     bool
	Hostname     string
	AppURL       string
	Version      int
	Organization Organization
	Tasks        []Task
	Secrets      []Secret
	Deployments  struct {
		Nodes []Deployment
	}
	Releases struct {
		Nodes []Release
	}
	IPAddresses struct {
		Nodes []IPAddress
	}
	Builds struct {
		Nodes []Build
	}
	Changes struct {
		Nodes []AppChange
	}
	Certificates struct {
		Nodes []AppCertificate
	}
	DeploymentStatus DeploymentStatus
	Certificate      AppCertificate
	Services         []Service
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
	Port         int
	InternalPort int
	Filters      []string
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
	ID      string
	Address string
	Type    string
}

type User struct {
	ID    string
	Name  string
	Email string
}

type Deployment struct {
	ID           string
	Number       int
	CurrentPhase string
	Description  string
	InProgress   bool
	Reason       string
	Status       string
	Trigger      string
	User         User
	CreatedAt    string
	UpdatedAt    string
	Release      struct {
		Version int
	}
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
	ID          string
	Version     int
	Reason      string
	Description string
	User        User
	CreatedAt   time.Time
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
	ID                     string
	AcmeDNSConfigured      bool
	CertificateAuthority   string
	CertificateRequestedAt time.Time
	DNSProvider            string
	DNSValidationTarget    string
	Hostname               string
	Source                 string
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
	Protocol     string   `json:"protocol"`
	Port         int      `json:"port"`
	InternalPort int      `json:"internalPort"`
	Handlers     []string `json:"handlers"`
}
