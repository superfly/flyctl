package api

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
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Runtime      string       `json:"runtime"`
	Status       string       `json:"status"`
	Version      int          `json:"version"`
	AppURL       string       `json:"appUrl"`
	Organization Organization `json:"organization"`
	Services     []Service    `json:"services"`
	Secrets      []string     `json:"secrets"`
	Deployments  struct {
		Nodes []Deployment
	}
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Service struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	Allications []Allocation `json:"allocations"`
}

type Allocation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Region    string `json:"region"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
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
	AppID   string        `json:"appId"`
	Secrets []SecretInput `json:"secrets"`
}

type SecretInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UnsetSecretsInput struct {
	AppID string   `json:"appId"`
	Keys  []string `json:"keys"`
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
