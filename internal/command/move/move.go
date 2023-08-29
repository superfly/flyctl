package move

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `The MOVE command will move an application to another
organization the current user belongs to.
`
		short = "Move an app to another organization"
		usage = "move <APPNAME>"
	)

	move := command.New(usage, short, long, apps.RunMove,
		command.RequireSession)
	move.Hidden = true
	move.Deprecated = "use `fly apps move` instead"

	move.Args = cobra.ExactArgs(1)

	flag.Add(move,
		flag.Yes(),
		flag.Org(),
	)

	return move
}
