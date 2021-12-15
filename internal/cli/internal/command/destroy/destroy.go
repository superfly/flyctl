package destroy

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/apps"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long = `The DESTROY command will remove an application 
from the Fly platform.
`
		short = "Permanently destroys an app"
		usage = "destroy [APPNAME]"
	)

	destroy := command.New(usage, short, long, apps.RunDestroy,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy,
		flag.Yes(),
	)

	return destroy
}
