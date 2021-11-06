package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newMove() *cobra.Command {
	const (
		long = `The APPS MOVE command will move an application to another 
organization the current user belongs to.
`

		short = "Move an app to another organization"

		usage = "move [APPNAME]"
	)

	move := command.New(usage, short, long, runMove,
		command.RequireSession)

	move.Args = cobra.ExactArgs(1)

	flag.Add(move,
		flag.Yes(),
		flag.Org(),
	)

	return move
}

func runMove(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
