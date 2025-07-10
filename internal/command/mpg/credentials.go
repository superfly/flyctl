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
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newCredentials() (cmd *cobra.Command) {
	const (
		long = `Display MPG credentials`

		short = long
		usage = "credentials"
	)

	cmd = command.New(usage, short, long, runCredentials, command.RequireSession, command.RequireUiex)
	cmd.Aliases = []string{"creds"}

	flag.Add(cmd,
		flag.Org(),
		flag.MPGCluster(),
	)

	return cmd
}

func runCredentials(ctx context.Context) (err error) {
	var (
		client     = flyutil.ClientFromContext(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		out        = iostreams.FromContext(ctx).Out
	)

	// Get cluster ID from flag - it's optional now
	clusterID := flag.GetMPGClusterID(ctx)
	if clusterID == "" {
		org, err := orgs.OrgFromFlagOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed fetching org: %w", err)
		}

		// For ui-ex requests we need the real org slug (resolve aliases like "personal")
		genqClient := client.GenqClient()
		var fullOrg *gql.GetOrganizationResponse
		if fullOrg, err = gql.GetOrganization(ctx, genqClient, org.Slug); err != nil {
			return fmt.Errorf("failed fetching org: %w", err)
		}

		// Now let user select a cluster from this organization
		selectedCluster, err := ClusterFromFlagOrSelect(ctx, fullOrg.Organization.RawSlug)
		if err != nil {
			return fmt.Errorf("failed fetching cluster: %w", err)
		}

		clusterID = selectedCluster.Id
	}

	response, err := uiexClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed fetching cluster: %w", err)
	}

	credentials := response.Credentials
	fmt.Fprintf(out, "%s\n", credentials.ConnectionUri)

	return
}
