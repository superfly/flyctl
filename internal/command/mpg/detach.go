package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach a managed Postgres cluster from an app"
		long  = short + ". " +
			`This command will remove the attachment record linking the app to the cluster.
Note: This does NOT remove any secrets from the app. Use 'fly secrets unset' to remove secrets.`
		usage = "detach <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runDetach,
		command.RequireSession,
		command.RequireAppName,
		requireMacaroonToken,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runDetach(ctx context.Context) error {
	var (
		clusterId = flag.FirstArg(ctx)
		appName   = appconfig.NameFromContext(ctx)
		io        = iostreams.FromContext(ctx)
	)

	// Get app details to determine which org it belongs to
	app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	appOrg, err := uiexutil.ClientFromContext(ctx).GetOrganization(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("failed retrieving organization for app %s: %w", appName, err)
	}

	appOrgSlug := appOrg.RawSlug
	if appOrgSlug != "" && clusterId == "" {
		fmt.Fprintf(io.Out, "Listing clusters in organization %s\n", appOrgSlug)
	}

	// Get cluster details
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterId, appOrgSlug)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	clusterOrgSlug := cluster.Organization.Slug

	// Verify that the app and cluster are in the same organization
	if !organizationSlugMatches(appOrg, clusterOrgSlug) {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization",
			appName, appOrgSlug, cluster.Id, clusterOrgSlug)
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunDetach(ctx, cluster.Id, appName)
	}

	return cmdv2.RunDetach(ctx, cluster.Id, appName)
}
