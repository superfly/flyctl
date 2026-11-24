package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/flyctl/internal/uiex/mpg"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach a Managed Postgres cluster from an app"
		long  = `Detach a Managed Postgres cluster from an app by removing the attachment
record that links them. The cluster, its data and its users are not
changed.

This does not remove the connection string secret from the app. Remove it
with ` + "`fly secrets unset DATABASE_URL`" + ` (or the variable name you used).

With no cluster ID, you pick one from the app's organization in a
terminal; without a terminal, the cluster ID is required.`
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
	appName := appconfig.NameFromContext(ctx)
	cluster, orgID, err := attachmentTarget(ctx, appName, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	ctx = requestauth.WithOrganizations(ctx, orgID)
	if cluster.Version == mpg.VersionV1 {
		return cmdv1.RunDetach(ctx, cluster.Id, appName)
	}

	return cmdv2.RunDetach(ctx, cluster.Id, appName)
}
