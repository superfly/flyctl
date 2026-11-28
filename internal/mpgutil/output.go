package mpgutil

// ClusterResult uses the same field names as cluster list and status JSON.
// It deliberately excludes credentials from cluster API responses.
type ClusterResult struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Organization ClusterOrganization `json:"organization"`
	Region       string              `json:"region"`
	Plan         string              `json:"plan"`
	Status       string              `json:"status"`
}

// ClusterOrganization preserves the organization keys used by list and status.
type ClusterOrganization struct {
	Name string `json:"Name"`
	Slug string `json:"Slug"`
}
