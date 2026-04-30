package mpg

import "github.com/superfly/fly-go"

type Version int

const (
	VersionV1 Version = iota
	VersionV2
)

// Unified cluster type that holds fields that are common
// across V1 and V2
type Cluster struct {
	Id            string                      `json:"id"`
	Name          string                      `json:"name"`
	Region        string                      `json:"region"`
	Status        string                      `json:"status"`
	Plan          string                      `json:"plan"`
	Disk          int                         `json:"disk"`
	Replicas      int                         `json:"replicas"`
	Organization  fly.Organization            `json:"organization"`
	IpAssignments ManagedClusterIpAssignments `json:"ip_assignments"`
	AttachedApps  []AttachedApp               `json:"attached_apps"`
	Version       Version                     `json:"version"`
	V2ClusterID   string                      `json:"v2_cluster_id"`
}

type ManagedClusterIpAssignments struct {
	Direct string `json:"direct"`
}

type AttachedApp struct {
	Name string `json:"name"`
	Id   int64  `json:"id"`
}
