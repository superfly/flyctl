package mpg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

const (
	localProxyPort = "16380"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql.

With --command or --file, psql stops at the first SQL error and skips its
startup file. See the examples for scripted usage.`

		example = `  fly mpg connect <CLUSTER_ID> -c 'select count(*) from orders'
  fly mpg connect <CLUSTER_ID> -f migrations.sql
  fly mpg connect <CLUSTER_ID> -c 'select * from orders' -- --csv -t
  # Piped SQL keeps psql defaults; enable ON_ERROR_STOP explicitly:
  cat queries.sql | fly mpg connect <CLUSTER_ID> -- -v ON_ERROR_STOP=1
  fly mpg connect <CLUSTER_ID> --database app --username reader -c 'select current_user'`

		short = "Connect to a MPG database using psql"
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession, requireMacaroonToken)

	cmd.Example = example
	cmd.SilenceUsage = true

	flag.Add(cmd,
		flag.StringArray{
			Name:        "command",
			Shorthand:   "c",
			Description: "The SQL command to run and exit (repeatable; mutually exclusive with --file)",
		},
		flag.StringArray{
			Name:        "file",
			Shorthand:   "f",
			Description: "The SQL file to run and exit (repeatable, - for stdin; mutually exclusive with --command)",
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
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if n := cmd.ArgsLenAtDash(); n >= 0 {
			args = args[:n]
		}

		return cobra.MaximumNArgs(1)(cmd, args)
	}

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	clusterID := clusterIDFromArgs(ctx)
	if flag.IsSpecified(ctx, "command") && flag.IsSpecified(ctx, "file") {
		return fmt.Errorf("--command and --file are mutually exclusive; use psql arguments after -- when both are needed in a specific order")
	}
	if (flag.IsSpecified(ctx, "command") || flag.IsSpecified(ctx, "file")) && clusterID == "" {
		return errors.New("cluster ID must be specified when using --command or --file")
	}

	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return flyerr.WithSuggestion(
			fmt.Errorf("psql is required to connect to MPG: %w", err),
			"Install the PostgreSQL client tools and add their bin directory to PATH: https://www.postgresql.org/download/",
		)
	}

	var orgSlug string

	cluster, orgSlug, err := ClusterFromArgOrSelect(ctx, clusterID, "")
	if err != nil {
		return err
	}

	// Resolve org slug alias for wireguard tunnel
	resolvedOrgSlug, err := AliasedOrganizationSlug(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("failed to resolve organization slug: %w", err)
	}

	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunConnect(ctx, cluster.Id, resolvedOrgSlug, psqlPath)
	}

	return cmdv2.RunConnect(ctx, cluster.Id, resolvedOrgSlug, localProxyPort, psqlPath)
}

// clusterIDFromArgs returns the cluster ID given before "--", if any.
// Everything after "--" belongs to psql.
func clusterIDFromArgs(ctx context.Context) string {
	if flag.FromContext(ctx).ArgsLenAtDash() == 0 {
		return ""
	}

	return flag.FirstArg(ctx)
}
