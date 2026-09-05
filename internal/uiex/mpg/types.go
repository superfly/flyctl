package mpg

import (
	"github.com/superfly/fly-go"
)

type Version int

const (
	VersionV1 Version = iota
	VersionV2
)

// Unified type for v1 and v2 MPG clusters
type Cluster struct {
	Id            string
	Name          string
	Region        string
	Status        string
	Plan          string
	Disk          int
	Replicas      int
	Organization  fly.Organization
	IpAssignments ManagedClusterIpAssignments
	AttachedApps  []AttachedApp
	Version       Version

	EligibleForV2Migration *bool // nil when the API did not report it
}

type ManagedClusterIpAssignments struct {
	Direct string `json:"direct"`
}

type AttachedApp struct {
	Name string `json:"name"`
	Id   int64  `json:"id"`
}
