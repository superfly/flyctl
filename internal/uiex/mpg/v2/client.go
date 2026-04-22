package v2

import (
	"context"

	"github.com/superfly/fly-go"
)

type ClientV2 interface {
	ListMPGRegions(ctx context.Context, orgSlug string) (ListMPGRegionsResponse, error)
	ListManagedClusters(ctx context.Context, orgSlug string, deleted bool) (ListManagedClustersResponse, error)
}

type ListMPGRegionsResponse struct {
	Data []MPGRegion `json:"data"`
}

type MPGRegion struct {
	Code      string `json:"code"`      // e.g., "fra"
	Available bool   `json:"available"` // Whether this region supports MPG
}

type ListManagedClustersResponse struct {
	Data []ManagedCluster `json:"data"`
}

type ManagedCluster struct {
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
}

type ManagedClusterIpAssignments struct {
	Direct string `json:"direct"`
}

type AttachedApp struct {
	Name string `json:"name"`
	Id   int64  `json:"id"`
}
