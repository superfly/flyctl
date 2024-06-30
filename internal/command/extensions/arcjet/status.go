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
		long  = short + "\n"

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
