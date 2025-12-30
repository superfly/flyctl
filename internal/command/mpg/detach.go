package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runDetach(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	var (
		clusterId = flag.FirstArg(ctx)
		appName   = appconfig.NameFromContext(ctx)
		client    = flyutil.ClientFromContext(ctx)
		io        = iostreams.FromContext(ctx)
	)

	// Get app details to determine which org it belongs to
	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	appOrgSlug := app.Organization.RawSlug
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
	if appOrgSlug != clusterOrgSlug {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization",
			appName, appOrgSlug, cluster.Id, clusterOrgSlug)
	}

	uiexClient := uiexutil.ClientFromContext(ctx)

	// Delete the attachment record
	_, err = uiexClient.DeleteAttachment(ctx, cluster.Id, appName)
	if err != nil {
		return fmt.Errorf("failed to detach: %w", err)
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s has been detached from %s\n", cluster.Id, appName)
	fmt.Fprintf(io.Out, "Note: This only removes the attachment record. Any secrets (like DATABASE_URL) are still set on the app.\n")
	fmt.Fprintf(io.Out, "Use 'fly secrets unset DATABASE_URL -a %s' to remove the connection string.\n", appName)

	return nil
}
