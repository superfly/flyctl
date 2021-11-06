package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

func newDestroy() *cobra.Command {
	const (
		long = `The APPS DESTROY command will remove an application 
from the Fly platform.
`

		short = "Permanently destroys an app"

		usage = "destroy [APPNAME]"
	)

	destroy := command.New(usage, short, long, runDestroy,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy, flag.Yes())

	return destroy
}

func runDestroy(ctx context.Context) error {
	return command.ErrNotImplementedYet
}
