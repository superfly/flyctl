package api

type Query struct {
	Apps        Nodes `json:"apps"`
	App         App   `json:"app"`
	CurrentUser User  `json:"currentUser"`
}

type Nodes struct {
	Nodes []App `json:"nodes"`
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
