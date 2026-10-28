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
		short = "Attach a managed Postgres cluster to an app"
		long  = short + ". " +
			`This command adds DATABASE_URL for pooled application connections and
 DIRECT_URL for direct connections on v2 clusters, such as ORM schema migrations.
 DIRECT_URL is skipped with a warning if it already exists or its endpoint is unavailable
 or invalid. Use --variable-name and --direct-variable-name to customize the secret names.
 When --variable-name is specified, pass --direct-variable-name to also set a direct secret.
 Pass --no-direct-url to skip it. Attach fails if the pooled secret or an explicitly named
 direct secret already exists.`
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
			Description: "The name of the environment variable for pooled connections; existing secrets are not overwritten",
		},
		flag.String{
			Name:        "direct-variable-name",
			Default:     "DIRECT_URL",
			Description: "The name of the environment variable for direct connections and migrations (v2 only); use --no-direct-url to skip; fails if the explicitly named secret already exists",
		},
		flag.Bool{
			Name:        "no-direct-url",
			Description: "Skip setting a direct connection URL",
			Default:     false,
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
	if err := validateAttachFlags(ctx); err != nil {
		return err
	}

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
		if flag.IsSpecified(ctx, "direct-variable-name") {
			return fmt.Errorf("--direct-variable-name is only supported for v2 clusters")
		}

		return cmdv1.RunAttach(ctx, cluster.Id)
	}

	return cmdv2.RunAttach(ctx, cluster.Id)
}

func validateAttachFlags(ctx context.Context) error {
	_, _, err := cmdv2.ResolveAttachSecretNames(ctx)

	return err
}
