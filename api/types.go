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

	// mutations
	CreateApp struct {
		App App
	}

	SetSecrets struct {
		Deployment Deployment
	}

	UnsetSecrets struct {
		Deployment Deployment
	}

	DeployImage struct {
		Deployment Deployment
	}
}

type App struct {
	ID           string
	Name         string
	Runtime      string
	Status       string
	Version      int
	AppURL       string
	Organization Organization
	Services     []Service
	Secrets      []string
	Deployments  struct {
		Nodes []Deployment
	}
	Releases struct {
		Nodes []Release
	}
}

type Organization struct {
	ID   string
	Name string
	Slug string
}

type Service struct {
	ID          string
	Name        string
	Status      string
	Allications []Allocation
}

type Allocation struct {
	ID        string
	Name      string
	Status    string
	Region    string
	CreatedAt string
	UpdatedAt string
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

type SetSecretsInput struct {
	AppID   string
	Secrets []SecretInput
}

type SecretInput struct {
	Key   string
	Value string
}

type UnsetSecretsInput struct {
	AppID string
	Keys  []string
}

type CreateAppInput struct {
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
