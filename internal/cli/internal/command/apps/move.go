package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newMove() *cobra.Command {
	move := command.FromDocstrings("apps.move", runMove,
		command.RequireSession)

	move.Args = cobra.ExactArgs(1)

	flag.Add(move, nil,
		flag.Yes(),
		flag.Org(),
	)

	return move
}

func runMove(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
