package api

type Apps struct {
	Apps Nodes `json:"apps"`
}

type Nodes struct {
	Nodes []App `json:"nodes"`
}
type App struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Runtime      string       `json:"runtime"`
	AppURL       string       `json:"appUrl"`
	Organization Organization `json:"organization"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
