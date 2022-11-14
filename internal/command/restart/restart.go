package restart

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
)

func newRestart() *cobra.Command {
	const (
		long  = `The APPS RESTART command will perform a rolling restart against all running VM's`
		short = "Restart an application"
		usage = "restart [APPNAME]"
	)

	cmd := command.New(usage, short, long, apps.RunRestart,
		command.RequireSession,
	)
	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(cmd,
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Will issue a restart against each Machine even if there are errors. ( Machines only )",
			Default:     false,
		},
		flag.Bool{
			Name:        "force-stop",
			Description: "Performs a force stop against the target Machine. ( Machines only )",
			Default:     false,
		},
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Restarts app without waiting for health checks. ( Machines only )",
			Default:     false,
		},
	)

	return cmd
}
