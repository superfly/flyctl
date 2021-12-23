package open

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func New() *cobra.Command {
	const (
		long = `Open browser to current deployed application. If an optional path is specified, this is appended to the
URL for deployed application
`
		short = "Open browser to current deployed application"

		usage = "open [RELATIVE_URI]"
	)

	cmd := command.New(usage, short, long, apps.RunOpen, command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}
