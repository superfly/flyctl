package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new Managed Postgres cluster"
		long  = short + "\n"
		usage = "create"
	)

	cmd := command.New(usage, short, long, runCreate,
		command.RequireSession,
		command.RequireUiex,
	)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres cluster",
		},
	)

	return cmd
}

func runCreate(ctx context.Context) error {
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	// Get the raw org slug for the UI-ex request
	genqClient := flyutil.ClientFromContext(ctx).GenqClient()
	fullOrg, err := gql.GetOrganization(ctx, genqClient, org.Slug)
	if err != nil {
		return fmt.Errorf("failed fetching org: %w", err)
	}

	name := flag.GetString(ctx, "name")
	if name == "" {
		if err := prompt.String(ctx, &name, "Choose a name for your Postgres cluster:", "", true); err != nil {
			return err
		}
	}

	// Prompt for region selection
	region, err := prompt.Region(ctx, !org.PaidPlan, prompt.RegionParams{
		Message: "Choose a primary region (can't be changed later):",
	})
	if err != nil {
		return err
	}

	// Create the request body with default values
	reqBody := &uiex.CreateManagedClusterRequest{
		Name:        name,
		Region:      region.Code,
		VolumeSize:  10,  // Default to 10GB
		ClusterSize: 1,   // Default to single node
	}

	// Get the UI-ex client and create the cluster
	uiexClient := uiexutil.ClientFromContext(ctx)
	response, err := uiexClient.CreateManagedCluster(ctx, fullOrg.Organization.RawSlug, reqBody)
	if err != nil {
		return err
	}

	// Print the result
	out := iostreams.FromContext(ctx).Out
	fmt.Fprintf(out, "Your Managed Postgres cluster %s is being created!\n", response.Data.Name)
	fmt.Fprintf(out, "\nTrack your cluster initialization progress at https://fly.io/dashboard/%s/managed_postgres/%s\n", fullOrg.Organization.RawSlug, response.Data.Id)

	return nil
}