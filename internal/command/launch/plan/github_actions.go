package plan

type GitHubActionsPlan struct {
	Deploy bool `json:"deploy"`
	Review bool `json:"review"`
}
