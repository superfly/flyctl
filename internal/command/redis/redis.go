package redis

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
)

// TODO: make internal once the open command has been deprecated
func New() (cmd *cobra.Command) {
	const (
		long  = `Launch and manage Redis databases managed by Upstash.com`
		short = long
	)

	cmd = command.New("redis", short, long, nil)

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDestroy(),
		newStatus(),
		newPlans(),
		newUpdate(),
		newConnect(),
		newDashboard(),
		newReset(),
	)

	return cmd
}

func GetExcludedRegions(ctx context.Context) (excludedRegions []string, err error) {
	client := client.FromContext(ctx).API().GenqClient

	_ = `# @genqlient
	query GetAddOnProvider($name: String!) {
		addOnProvider(name: $name) {
			id
			name
			excludedRegions {
				code
			}
		}
	}
	`

	response, err := gql.GetAddOnProvider(ctx, client, "upstash_redis")

	if err != nil {
		return nil, err
	}

	for _, region := range response.AddOnProvider.ExcludedRegions {
		excludedRegions = append(excludedRegions, region.Code)
	}

	return
}
