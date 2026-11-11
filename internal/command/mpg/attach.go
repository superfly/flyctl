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

func newAttach() *cobra.Command {
	const (
		short = "Attach a Managed Postgres cluster to an app"
		long  = `Attach a Managed Postgres cluster to an app by setting a secret on the app
(DATABASE_URL unless --variable-name says otherwise) that holds the
cluster's connection string. The app and the cluster must be in the same
organization, and the command fails if the app already has a secret with
that name.

With no cluster ID, you pick one from the app's organization in a
terminal; without a terminal, the cluster ID is required. In a terminal,
you're also asked which user and database to use, with the option to
create new ones, unless you pass --username and --database. Without a
terminal and without those flags, the cluster's default credentials and
database are used.

Remove the link with ` + "`fly mpg detach`" + `.`
		usage = "attach <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
		requireMacaroonToken,
	)
	// cmd.Args = cobra.ExactArgs(1)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The name of the environment variable that will be added to the attached app",
		},
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to connect as",
		},
	)

	return cmd
}

func runAttach(ctx context.Context) error {
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

	// Get cluster details to determine which org it belongs to
	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterId, appOrgSlug)
	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}

	clusterOrgSlug := cluster.Organization.Slug

	// Verify that the app and cluster are in the same organization
	if !organizationSlugMatches(appOrg, clusterOrgSlug) {
		return fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization to attach",
			appName, appOrgSlug, cluster.Id, clusterOrgSlug)
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunAttach(ctx, cluster.Id)
	}

	return cmdv2.RunAttach(ctx, cluster.Id)
}
