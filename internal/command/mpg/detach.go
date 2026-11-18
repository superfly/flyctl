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
