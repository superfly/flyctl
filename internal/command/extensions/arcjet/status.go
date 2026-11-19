package arcjet

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func status() *cobra.Command {
	const (
		short = "Show details about an Arcjet site setup"
		long  = `Show an Arcjet site's name, primary region and status. Name the site as
an argument, or run the command in an app directory (or with ` + "`--app`" + `) to
use the app's site. When the site is found through an app, the app's
name is shown too.`

		usage = "status [name]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession, command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		extensions_core.SharedFlags,
	)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	return extensions_core.Status(ctx, gql.AddOnTypeArcjet)
}
