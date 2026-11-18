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
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach a managed Postgres cluster to an app"
		long  = short + ". " +
			`This command will add a secret to the specified app
 containing the connection string for the database.`
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

// attachmentTarget resolves an explicit cluster before narrowing its credentials to
// the app's organization. An org-filtered list can use that organization's token immediately.
func attachmentTarget(ctx context.Context, appName, clusterID string) (*mpg.Cluster, uint64, error) {
	app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	org, err := uiexutil.AppOrganization(ctx, app)
	if err != nil {
		return nil, 0, err
	}

	if org.RawSlug != "" && clusterID == "" {
		fmt.Fprintf(iostreams.FromContext(ctx).Out, "Listing clusters in organization %s\n", org.RawSlug)
		ctx = requestauth.WithOrganizations(ctx, org.InternalNumericID)
	}

	cluster, _, err := ClusterFromArgOrSelect(ctx, clusterID, org.RawSlug)
	if err != nil {
		return nil, 0, fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
	}
	if !organizationSlugMatches(org, cluster.Organization.Slug) {
		return nil, 0, fmt.Errorf("app %s is in organization %s, but cluster %s is in organization %s. They must be in the same organization",
			appName, org.RawSlug, cluster.Id, cluster.Organization.Slug)
	}

	// A missing org slug can match an empty alias. Preserve that existing
	// behavior without treating an empty slug match as proof of ownership.
	if cluster.Organization.Slug == "" {
		return cluster, 0, nil
	}

	return cluster, org.InternalNumericID, nil
}

func runAttach(ctx context.Context) error {
	cluster, orgID, err := attachmentTarget(ctx, appconfig.NameFromContext(ctx), flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	ctx = requestauth.WithOrganizations(ctx, orgID)
	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunAttach(ctx, cluster.Id)
	}

	return cmdv2.RunAttach(ctx, cluster.Id)
}
