package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new Redis instance`

		short = long
		usage = "create"
	)

	cmd = command.New(usage, short, long, runCreate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API().GenqClient
	)

	org, err := prompt.Org(ctx)

	if err != nil {
		return err
	}

	//region, err := prompt.Region()
	var index int
	var promptOptions []string

	result, err := gql.ListAddOnPlans(ctx, client)

	if err != nil {
		return
	}

	for _, plan := range result.AddOnPlans.Nodes {
		promptOptions = append(promptOptions, fmt.Sprintf("%s: %s Max Data Size, $%d/month/region", plan.DisplayName, plan.MaxDataSize, plan.PricePerMonth))
	}

	err = prompt.Select(ctx, &index, "Select a Redis plan", "", promptOptions...)

	if err != nil {
		return fmt.Errorf("failed to select a plan: %w", err)
	}

	url, err := ProvisionRedis(ctx, org, result.AddOnPlans.Nodes[index].Id, "us-east-1")

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Redis instance is available to apps in the %s organization at %s\n", org.Slug, url)

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization, planId string, region string) (publicUrl string, err error) {
	client := client.FromContext(ctx).API().GenqClient

	_ = `# @genqlient
  mutation ProvisionAddOn($organizationId: ID!, $region: String!, $planId: ID!) {
		provisionAddOn(input: {organizationId: $organizationId, region: $region, type: upstash_redis, planId: $planId}) {
			addOn {
				id
				publicUrl
			}
		}
  }
`

	response, err := gql.ProvisionAddOn(ctx, client, org.ID, region, planId)

	if err != nil {
		return
	}

	return response.ProvisionAddOn.AddOn.PublicUrl, nil
}
