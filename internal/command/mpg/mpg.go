package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func New() *cobra.Command {
	const (
		short = `Manage Managed Postgres clusters.`

		long = short + "\n"
	)

	cmd := command.New("managed-postgres", short, long, nil)

	cmd.Aliases = []string{"mpg"}

	cmd.AddCommand(
		newProxy(),
		newConnect(),
		newAttach(),
	)

	return cmd
}

// ClusterFromFlagOrSelect retrieves the cluster ID from the --cluster flag.
// If the flag is not set, it prompts the user to select a cluster from the available ones for the given organization.
func ClusterFromFlagOrSelect(ctx context.Context, orgSlug string) (*uiex.ManagedCluster, error) {
	clusterID := flag.GetMPGClusterID(ctx)
	uiexClient := uiexutil.ClientFromContext(ctx)

	clustersResponse, err := uiexClient.ListManagedClusters(ctx, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving postgres clusters: %w", err)
	}

	if len(clustersResponse.Data) == 0 {
		return nil, fmt.Errorf("no managed postgres clusters found in organization %s", orgSlug)
	}

	if clusterID != "" {
		// If a cluster ID is provided via flag, find it
		for i := range clustersResponse.Data {
			if clustersResponse.Data[i].Id == clusterID {
				return &clustersResponse.Data[i], nil
			}
		}
		return nil, fmt.Errorf("managed postgres cluster %q not found in organization %s", clusterID, orgSlug)
	} else {
		// Otherwise, prompt the user to select a cluster
		var options []string
		for _, cluster := range clustersResponse.Data {
			options = append(options, fmt.Sprintf("%s (%s)", cluster.Name, cluster.Region))
		}

		var index int
		selectErr := prompt.Select(ctx, &index, "Select a Postgres cluster", "", options...)
		if selectErr != nil {
			return nil, selectErr
		}
		return &clustersResponse.Data[index], nil
	}
}
