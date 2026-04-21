package utils

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

type ManagedCluster struct {
	Id           string           `json:"id"`
	Name         string           `json:"name"`
	Region       string           `json:"region"`
	Status       string           `json:"status"`
	Plan         string           `json:"plan"`
	Disk         int              `json:"disk"`
	Replicas     int              `json:"replicas"`
	Organization fly.Organization `json:"organization"`
	// IpAssignments ManagedClusterIpAssignments `json:"ip_assignments"`
	// AttachedApps  []AttachedApp               `json:"attached_apps"`
}

type ListManagedClustersResponse struct {
	Data []ManagedCluster `json:"data"`
}

// ClusterFromArgOrSelect retrieves the cluster if the cluster ID is passed in
// otherwise it prompts the user to select a cluster from the available ones for
// the given organization.
// It prompts for the org if the org slug is not provided.
func ClusterFromArgOrSelect(ctx context.Context, clusterID, orgSlug string) (*ManagedCluster, string, error) {
	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return nil, "", err
		}

		orgSlug = org.RawSlug
	}

	// Fetch V1 clusters
	mpgv1Client := mpgv1.ClientFromContext(ctx)
	clustersResponse, err := mpgv1Client.ListManagedClusters(ctx, orgSlug, false)
	if err != nil {
		return nil, orgSlug, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	// // Fetch V2 clusters
	// v2Client := mpgv2.Client{
	// 	Client: uiexClient,
	// }
	// clustersV2, err := v2Client.ListManagedClusters(ctx, orgSlug, false)
	// if err != nil {
	// 	return nil, orgSlug, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	// }

	// if len(clustersV1.Data) == 0 && len(clustersV2.Data) == 0 {
	// 	return nil, orgSlug, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	// }

	// clusters := slices.Concat(clustersV1.Data, clustersV2.Data)
	clusters := make([]*ManagedCluster, 0, len(clustersResponse.Data))
	for _, cluster := range clustersResponse.Data {
		clusters = append(clusters, &ManagedCluster{
			Id:           cluster.Id,
			Name:         cluster.Name,
			Region:       cluster.Region,
			Status:       cluster.Status,
			Plan:         cluster.Plan,
			Disk:         cluster.Disk,
			Replicas:     cluster.Replicas,
			Organization: cluster.Organization,
		})
	}

	// If a cluster ID is provided via flag, find it
	if clusterID != "" {
		for _, cluster := range clusters {
			if cluster.Id == clusterID {
				return cluster, orgSlug, nil
			}
		}

		return nil, orgSlug, fmt.Errorf("managed postgres cluster %q not found in organization %s", clusterID, orgSlug)
	}

	// Otherwise, prompt the user to select a cluster
	var options []string
	for _, cluster := range clusters {
		options = append(options, fmt.Sprintf("%s [%s] (%s)", cluster.Name, cluster.Id, cluster.Region))
	}

	var index int
	selectErr := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...)
	if selectErr != nil {
		return nil, orgSlug, selectErr
	}

	return clusters[index], orgSlug, nil
}
